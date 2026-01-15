FROM golang:1.25-alpine AS builder

WORKDIR /app
# Install build dependencies for CGO (sqlite, duckdb)
RUN apk add --no-cache gcc g++ musl-dev

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Build statically linked binary
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-linkmode external -extldflags=-static" -o consumer ./cmd/consumer

FROM alpine:latest
WORKDIR /app

# Install ca-certificates for HTTPS
RUN apk add --no-cache ca-certificates

COPY --from=builder /app/consumer .

# Create DB directory
RUN mkdir -p /data
ENV DATA_DIR=/data

EXPOSE 8080
CMD ["./consumer"]
