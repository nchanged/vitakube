#!/bin/bash
set -e

# Configuration
IMAGE_NAME="nchanged/vita-consumer"
VERSION="0.1.0"
DOCKERFILE_PATH="packages/vita-consumer/Dockerfile"
BUILD_CONTEXT="packages/vita-consumer"
HELM_RELEASE="vita-agent" # Shared release
HELM_CHART="./chart"
NAMESPACE="vitakube"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

print_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
print_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
print_error() { echo -e "${RED}[ERROR]${NC} $1"; }

build() {
    print_info "Building Docker image: ${IMAGE_NAME}:${VERSION}"
    if [ ! -f "${DOCKERFILE_PATH}" ]; then
        print_error "Dockerfile not found at ${DOCKERFILE_PATH}"
        exit 1
    fi
    docker build -t "${IMAGE_NAME}:${VERSION}" -f "${DOCKERFILE_PATH}" "${BUILD_CONTEXT}"
    docker tag "${IMAGE_NAME}:${VERSION}" "${IMAGE_NAME}:latest"
    print_info "✓ Build complete"
}

push() {
    print_info "Pushing Docker image..."
    docker push "${IMAGE_NAME}:${VERSION}"
    docker push "${IMAGE_NAME}:latest"
    print_info "✓ Push complete"
}

deploy() {
    print_info "Deploying to Kubernetes via Helm..."
    
    kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

    helm upgrade --install "${HELM_RELEASE}" "${HELM_CHART}" \
        --namespace "${NAMESPACE}" \
        --create-namespace \
        --set consumer.image.repository="${IMAGE_NAME}" \
        --set consumer.image.pullPolicy=Always \
        --set consumer.image.tag="${VERSION}"

    print_info "Waiting for deployment..."
    kubectl rollout status deployment/${HELM_RELEASE}-consumer -n "${NAMESPACE}" --timeout=60s || true
    print_info "✓ Deploy complete"
}

dev() {
    print_info "Running locally (DEV mode)..."
    cd packages/vita-consumer
    export DATA_DIR=".data"
    mkdir -p .data
    go mod tidy
    go run cmd/consumer/main.go
}

usage() {
    echo "Usage: $0 [--build] [--push] [--deploy] [--dev] [--all]"
}

main() {
    if [ $# -eq 0 ]; then usage; exit 1; fi
    
    DO_BUILD=false; DO_PUSH=false; DO_DEPLOY=false; DO_DEV=false

    while [[ $# -gt 0 ]]; do
        case $1 in
            --build) DO_BUILD=true; shift ;;
            --push) DO_PUSH=true; shift ;;
            --deploy) DO_DEPLOY=true; shift ;;
            --dev) DO_DEV=true; shift ;;
            --all) DO_BUILD=true; DO_PUSH=true; DO_DEPLOY=true; shift ;;
            *) usage; exit 1 ;;
        esac
    done

    if [ "$DO_DEV" = true ]; then dev; exit 0; fi
    if [ "$DO_BUILD" = true ]; then build; fi
    if [ "$DO_PUSH" = true ]; then push; fi
    if [ "$DO_DEPLOY" = true ]; then deploy; fi
}

main "$@"
