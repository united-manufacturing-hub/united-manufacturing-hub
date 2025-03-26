# Copyright 2025 UMH Systems GmbH
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
# --- Build Stage ---
FROM golang:1.24-alpine as build
WORKDIR /go/src/github.com/united-manufacturing-hub/united-manufacturing-hub
RUN apk add --no-cache gcc musl-dev
COPY ./go.mod ./umh-core/
WORKDIR /go/src/github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o /go/bin/umh-core cmd/main.go

# --- Downloader Stage (slow downloads) ---
FROM alpine:3.21 as downloader
RUN apk add --no-cache bash curl jq tzdata ca-certificates xz wget

ARG S6_OVERLAY_VERSION=3.2.0.2
ARG BENTHOS_UMH_VERSION=0.6.2
ARG REDPANDA_VERSION=24.3.8
ARG TARGETARCH=amd64

# s6-overlay noarch
ADD https://github.com/just-containers/s6-overlay/releases/download/v${S6_OVERLAY_VERSION}/s6-overlay-noarch.tar.xz /tmp/
RUN tar -C / -Jxpf /tmp/s6-overlay-noarch.tar.xz && rm /tmp/s6-overlay-noarch.tar.xz

# s6-overlay arch-specific
RUN case "${TARGETARCH}" in \
    "amd64") S6_ARCH="x86_64" ;; \
    "arm64") S6_ARCH="aarch64" ;; \
    *) echo "Unsupported architecture: ${TARGETARCH}" && exit 1 ;; \
    esac && \
    wget -O /tmp/s6-overlay-arch.tar.xz "https://github.com/just-containers/s6-overlay/releases/download/v${S6_OVERLAY_VERSION}/s6-overlay-${S6_ARCH}.tar.xz" && \
    tar -C / -Jxpf /tmp/s6-overlay-arch.tar.xz && rm /tmp/s6-overlay-arch.tar.xz

# syslogd overlay
ADD https://github.com/just-containers/s6-overlay/releases/download/v${S6_OVERLAY_VERSION}/syslogd-overlay-noarch.tar.xz /tmp/
RUN tar -C / -Jxpf /tmp/syslogd-overlay-noarch.tar.xz && rm /tmp/syslogd-overlay-noarch.tar.xz

# Benthos installation
RUN case "${TARGETARCH}" in \
    "amd64") BENTHOS_ARCH="amd64" ;; \
    "arm64") BENTHOS_ARCH="arm64" ;; \
    *) echo "Unsupported architecture: ${TARGETARCH}" && exit 1 ;; \
    esac && \
    curl -sSL -o /usr/local/bin/benthos "https://github.com/united-manufacturing-hub/benthos-umh/releases/download/v${BENTHOS_UMH_VERSION}/benthos-linux-${BENTHOS_ARCH}" && \
    chmod +x /usr/local/bin/benthos

# Redpanda installation
RUN case "${TARGETARCH}" in \
    "amd64") REDPANDA_ARCH="amd64" ;; \
    "arm64") REDPANDA_ARCH="arm64" ;; \
    *) echo "Unsupported architecture: ${TARGETARCH}" && exit 1 ;; \
    esac && \
    mkdir -p /tmp/redpanda /opt/redpanda && \
    curl -sSL -o /tmp/redpanda/redpanda.tar.gz "https://dl.redpanda.com/nzc4ZYQK3WRGd9sy/redpanda/raw/names/redpanda-${REDPANDA_ARCH}/versions/${REDPANDA_VERSION}/redpanda-${REDPANDA_VERSION}-${REDPANDA_ARCH}.tar.gz" && \
    tar -xzf /tmp/redpanda/redpanda.tar.gz -C /opt/redpanda && rm /tmp/redpanda/redpanda.tar.gz && \
    mkdir -p /data/redpanda /data/redpanda/coredump && \
    chmod +x /opt/redpanda/bin/redpanda

# --- Final Stage ---
FROM downloader
# Copy redpanda config
COPY ./redpanda/redpanda.yaml /opt/redpanda/etc/redpanda/redpanda.yaml
# Copy built application
COPY --from=build /go/bin/umh-core /usr/local/bin/umh-core
RUN chmod +x /usr/local/bin/umh-core && \
    ls -la /usr/local/bin/umh-core && \
    test -x /usr/local/bin/umh-core || (echo "Binary not executable" && exit 1)
# Copy s6 service configuration
COPY ./s6-base/s6-rc.d /etc/s6-overlay/s6-rc.d/
RUN chmod +x /etc/s6-overlay/s6-rc.d/umh-core/run
# Set environment variables and volume
ENV S6_KEEP_ENV=1 \
    S6_BEHAVIOUR_IF_STAGE2_FAILS=2 \
    S6_CMD_WAIT_FOR_SERVICES=1 \
    S6_SERVICES_READYTIME=50 \
    S6_SYNC_DISKS=1 \
    S6_LOGGING_SCRIPT="n20 s1000000 T"
VOLUME ["/data"]
ENTRYPOINT ["/init"]
CMD []
