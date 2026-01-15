#!/bin/bash
set -e

# Configuration
IMAGE_NAME="nchanged/vita-agent"
VERSION="0.1.0"
DOCKERFILE_PATH="packages/vita-agent/Dockerfile"
BUILD_CONTEXT="packages/vita-agent"
HELM_RELEASE="vita-agent"
HELM_CHART="./chart"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Helper functions
print_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Build function
build() {
    print_info "Building Docker image: ${IMAGE_NAME}:${VERSION}"
    
    if [ ! -f "${DOCKERFILE_PATH}" ]; then
        print_error "Dockerfile not found at ${DOCKERFILE_PATH}"
        exit 1
    fi
    
    docker build -t "${IMAGE_NAME}:${VERSION}" -f "${DOCKERFILE_PATH}" "${BUILD_CONTEXT}"
    docker tag "${IMAGE_NAME}:${VERSION}" "${IMAGE_NAME}:latest"
    
    print_info "✓ Build complete"
    print_info "  Tagged: ${IMAGE_NAME}:${VERSION}"
    print_info "  Tagged: ${IMAGE_NAME}:latest"
}

# Push function
push() {
    print_info "Pushing Docker image to Docker Hub"
    
    # Check if logged in to Docker Hub (optional check)
    if ! docker info 2>/dev/null | grep -q "Username"; then
        print_warn "Could not detect Docker Hub username (you might still be logged in via credential helper)"
    fi
    
    print_info "Pushing ${IMAGE_NAME}:${VERSION}..."
    if ! docker push "${IMAGE_NAME}:${VERSION}"; then
        print_error "Failed to push ${IMAGE_NAME}:${VERSION}. Are you logged in? (Try: docker login)"
        exit 1
    fi
    
    print_info "Pushing ${IMAGE_NAME}:latest..."
    docker push "${IMAGE_NAME}:latest"
    
    print_info "✓ Push complete"
}

# Reload function
reload() {
    print_info "Reloading vita-agent in Kubernetes"
    
    NAMESPACE_FLAG=""
    if [ ! -z "$NAMESPACE" ]; then
        NAMESPACE_FLAG="--namespace ${NAMESPACE}"
        print_info "Target Namespace: ${NAMESPACE}"
        
        # Create namespace if it doesn't exist (only on install)
        if ! kubectl get namespace "${NAMESPACE}" >/dev/null 2>&1; then
             print_info "Creating namespace ${NAMESPACE}..."
             kubectl create namespace "${NAMESPACE}"
        fi
    fi

    # Check if Helm release exists
    if ! helm list -A | grep -q "${HELM_RELEASE}"; then
        print_warn "Helm release '${HELM_RELEASE}' not found. Installing..."
        helm install "${HELM_RELEASE}" "${HELM_CHART}" ${NAMESPACE_FLAG}
    else
        print_info "Upgrading Helm release..."
        helm upgrade "${HELM_RELEASE}" "${HELM_CHART}" ${NAMESPACE_FLAG} --force
    fi
    
    print_info "Waiting for DaemonSet rollout..."
    # Try different name variations if one fails, or search by label
    if ! kubectl rollout status daemonset/${HELM_RELEASE} ${NAMESPACE_FLAG} --timeout=60s 2>/dev/null; then
         kubectl rollout status daemonset/${HELM_RELEASE}-vita-agent ${NAMESPACE_FLAG} --timeout=60s || true
    fi
    
    print_info "✓ Reload complete"
    print_info ""
    print_info "View logs with:"
    print_info "  kubectl logs -f daemonset/${HELM_RELEASE} ${NAMESPACE_FLAG} --all-containers=true --max-log-requests=10"
}

# Usage function
usage() {
    cat << EOF
Usage: $0 [OPTIONS]

Operations script for vita-agent build and deployment.

OPTIONS:
    --build         Build the Docker image
    --push          Push the Docker image to Docker Hub (nchanged/vita-agent)
    --deploy        Deploy/Reload the Helm chart in Kubernetes (alias: --reload)
    --namespace <ns> Specify Kubernetes namespace (default: vitakube)
    --all           Run build, push, and reload in sequence
    -h, --help      Show this help message

EXAMPLES:
    # Build only
    $0 --build

    # Build and push
    $0 --build --push

    # Deploy to 'monitoring' namespace
    $0 --reload --namespace monitoring

    # Full workflow to specific namespace
    $0 --all --namespace custom-ns

CONFIGURATION:
    Image:    ${IMAGE_NAME}:${VERSION}
    Chart:    ${HELM_CHART}
    Release:  ${HELM_RELEASE}

EOF
}

# Main logic
main() {
    if [ $# -eq 0 ]; then
        usage
        exit 1
    fi

    DO_BUILD=false
    DO_PUSH=false
    DO_RELOAD=false
    NAMESPACE="vitakube"

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            --build)
                DO_BUILD=true
                shift
                ;;
            --push)
                DO_PUSH=true
                shift
                ;;
            --reload|--deploy)
                DO_RELOAD=true
                shift
                ;;
            --namespace)
                NAMESPACE="$2"
                shift 2
                ;;
            --all)
                DO_BUILD=true
                DO_PUSH=true
                DO_RELOAD=true
                shift
                ;;
            -h|--help)
                usage
                exit 0
                ;;
            *)
                print_error "Unknown option: $1"
                usage
                exit 1
                ;;
        esac
    done

    # Execute requested actions
    if [ "$DO_BUILD" = true ]; then
        build
    fi

    if [ "$DO_PUSH" = true ]; then
        push
    fi

    if [ "$DO_RELOAD" = true ]; then
        reload
    fi

    print_info ""
    print_info "🎉 All operations completed successfully!"
}

# Run main
main "$@"
