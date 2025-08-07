# Multi-stage build for s6 + benthos test environment

# Stage 1: Get UMH Benthos binary
FROM ghcr.io/united-manufacturing-hub/benthos-umh:0.10.2 AS benthos-source

# Stage 2: Build Go application
FROM golang:1.22-alpine AS go-builder
WORKDIR /build

# Copy go.mod and go.sum (if it exists)
COPY go.mod ./
COPY go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY cmd/ ./cmd/
COPY internal/ ./internal/

# Build the application
RUN go build -o s6-benthos-test ./cmd/main.go

# Stage 3: Final runtime image
FROM alpine:3.19

# Install s6-overlay v3 and other dependencies
ENV S6_OVERLAY_VERSION=3.1.6.2
ADD https://github.com/just-containers/s6-overlay/releases/download/v${S6_OVERLAY_VERSION}/s6-overlay-noarch.tar.xz /tmp
ADD https://github.com/just-containers/s6-overlay/releases/download/v${S6_OVERLAY_VERSION}/s6-overlay-x86_64.tar.xz /tmp

RUN apk add --no-cache \
    bash \
    curl \
    ca-certificates \
    tzdata \
    tar \
    xz \
    && tar -C / -Jxpf /tmp/s6-overlay-noarch.tar.xz \
    && tar -C / -Jxpf /tmp/s6-overlay-x86_64.tar.xz \
    && rm -f /tmp/*.tar.xz

# Copy benthos binary from UMH container
COPY --from=benthos-source /benthos /usr/local/bin/benthos
RUN chmod +x /usr/local/bin/benthos

# Copy built Go application
COPY --from=go-builder /build/s6-benthos-test /test/s6-benthos-test

# Create necessary directories
RUN mkdir -p \
    /run/service \
    /data/services \
    /data/logs \
    /data/benthos \
    /test

# Set up s6-overlay environment
ENV S6_CMD_WAIT_FOR_SERVICES_MAXTIME=0
ENV S6_VERBOSITY=2

WORKDIR /test

# Set entrypoint to s6 init
ENTRYPOINT ["/init"]

# Default command runs our test program
CMD ["./s6-benthos-test"]