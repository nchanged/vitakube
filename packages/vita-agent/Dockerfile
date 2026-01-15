# Build stage - use musl for static linking
FROM rust:1.85-alpine as builder

# Install musl-dev for static linking
RUN apk add --no-cache musl-dev

WORKDIR /app

# Copy manifests
COPY Cargo.toml ./

# Copy source code
COPY src ./src

# Build with musl target for full static linking
# Note: On Alpine, the default target is already musl
RUN cargo build --release

# Runtime stage - FROM scratch for minimal image
FROM scratch

# Copy CA certificates from builder (needed for HTTPS to k8s API)
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy the statically linked binary
COPY --from=builder /app/target/release/vita-agent /vita-agent

# Set environment variables
ENV RUST_LOG=info

# Run the binary
ENTRYPOINT ["/vita-agent"]
