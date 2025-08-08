FROM golang:1.24-alpine as builder
WORKDIR /build
RUN apk add build-base
COPY go.mod .
COPY go.sum .
RUN go mod download
COPY "cmd" "cmd"
RUN CGO_ENABLED=1 go build -race -ldflags "-s -w" -o s6-rc-poc cmd/main.go

FROM alpine as runner
ARG S6_OVERLAY_VERSION=3.2.1.0
# Install s6-overlay
ADD https://github.com/just-containers/s6-overlay/releases/download/v${S6_OVERLAY_VERSION}/s6-overlay-noarch.tar.xz /tmp
RUN tar -C / -Jxpf /tmp/s6-overlay-noarch.tar.xz
ADD https://github.com/just-containers/s6-overlay/releases/download/v${S6_OVERLAY_VERSION}/s6-overlay-x86_64.tar.xz /tmp
RUN tar -C / -Jxpf /tmp/s6-overlay-x86_64.tar.xz

# Copy static service configuration
COPY service/s6-rc.d /etc/s6-overlay/s6-rc.d/
# Enable poc run
RUN chmod +x /etc/s6-overlay/s6-rc.d/poc/run

# Copy our binary
COPY --from=builder --chmod=755 /build/s6-rc-poc /opt/s6-rc-poc

ENV S6_KEEP_ENV=1 \
    S6_BEHAVIOUR_IF_STAGE2_FAILS=2 \
    S6_CMD_WAIT_FOR_SERVICES=1 \
    S6_SERVICES_READYTIME=50 \
    S6_SYNC_DISKS=1 \
    S6_LOGGING_SCRIPT="n5 s10000000 T"

ENTRYPOINT ["/init"]
CMD []

