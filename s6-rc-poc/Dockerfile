FROM golang:1.24-alpine as builder
WORKDIR /build
RUN apk add build-base
COPY go.mod .
COPY go.sum .
RUN go mod download
COPY "cmd" "cmd"
RUN CGO_ENABLED=1 go build -race -ldflags "-s -w" -o s6-rc-poc cmd/main.go

FROM alpine as runner
# Install s6 and s6-rc directly from Alpine packages
RUN apk add --no-cache s6 s6-rc execline

# Create necessary directories
RUN mkdir -p /run/service \
    && mkdir -p /etc/s6-rc/source \
    && mkdir -p /etc/s6-rc/compiled

# Copy our binary
COPY --from=builder --chmod=755 /build/s6-rc-poc /opt/s6-rc-poc

# Copy initial s6-rc source structure
COPY s6-rc-source /etc/s6-rc/source/

# Simple entrypoint - just run our Go program
ENTRYPOINT ["/opt/s6-rc-poc"]
CMD []