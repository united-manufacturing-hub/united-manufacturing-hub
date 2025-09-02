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
ARG GOLANG_VERSION=MUST_BE_SET_BY_MAKEFILE
ARG ALPINE_VERSION=MUST_BE_SET_BY_MAKEFILE
ARG NMAP_VERSION=MUST_BE_SET_BY_MAKEFILE

# --- Cache Stage ---
FROM scratch as cache
COPY .docker-cache/s6-overlay/ /s6-overlay/
COPY .docker-cache/benthos/ /benthos/
COPY .docker-cache/redpanda/amd64/ /redpanda/amd64/
COPY .docker-cache/redpanda/arm64/ /redpanda/arm64/


# --- Downloader Stage (using cached files) ---
FROM alpine:${ALPINE_VERSION} as downloader
RUN apk add --no-cache bash curl jq tzdata ca-certificates xz bc

ARG S6_OVERLAY_VERSION=MUST_BE_SET_BY_MAKEFILE
ARG BENTHOS_UMH_VERSION=MUST_BE_SET_BY_MAKEFILE
ARG REDPANDA_VERSION=MUST_BE_SET_BY_MAKEFILE
ARG NMAP_VERSION=MUST_BE_SET_BY_MAKEFILE
ARG DLV_VERSION=MUST_BE_SET_BY_MAKEFILE
ARG TARGETARCH=amd64

# Copy cached files from cache stage
COPY --from=cache /s6-overlay/s6-overlay-noarch.tar.xz /tmp/
COPY --from=cache /s6-overlay/syslogd-overlay-noarch.tar.xz /tmp/

# s6-overlay noarch
RUN tar -C / -Jxpf /tmp/s6-overlay-noarch.tar.xz && rm /tmp/s6-overlay-noarch.tar.xz

# s6-overlay arch-specific
COPY --from=cache /s6-overlay/s6-overlay-aarch64.tar.xz /s6-overlay/s6-overlay-aarch64.tar.xz
COPY --from=cache /s6-overlay/s6-overlay-x86_64.tar.xz /s6-overlay/s6-overlay-x86_64.tar.xz
RUN S6_ARCH=$([ "${TARGETARCH}" = "arm64" ] && echo "aarch64" || echo "x86_64") && \
    cp /s6-overlay/s6-overlay-${S6_ARCH}.tar.xz /tmp/ && \
    tar -C / -Jxpf /tmp/s6-overlay-${S6_ARCH}.tar.xz && rm /tmp/s6-overlay-${S6_ARCH}.tar.xz

# syslogd overlay
RUN tar -C / -Jxpf /tmp/syslogd-overlay-noarch.tar.xz && rm /tmp/syslogd-overlay-noarch.tar.xz

# Benthos installation
COPY --from=cache /benthos/benthos-linux-amd64 /benthos/benthos-linux-amd64
COPY --from=cache /benthos/benthos-linux-arm64 /benthos/benthos-linux-arm64
RUN case "${TARGETARCH}" in \
    "amd64") BENTHOS_ARCH="amd64" ;; \
    "arm64") BENTHOS_ARCH="arm64" ;; \
    *) echo "Unsupported architecture: ${TARGETARCH}" && exit 1 ;; \
    esac && \
    cp /benthos/benthos-linux-${BENTHOS_ARCH} /usr/local/bin/benthos && \
    chmod +x /usr/local/bin/benthos

# Redpanda installation
COPY --from=cache /redpanda/${TARGETARCH}/ /opt/
RUN chmod +x /opt/redpanda/bin/redpanda

# Install nmap
RUN apk add --no-cache nmap=${NMAP_VERSION}

# Install strace
RUN apk add --no-cache strace

# Cleanup all the files we don't need
RUN rm -rf /s6-overlay /benthos /redpanda

FROM golang:${GOLANG_VERSION}-alpine as build
ARG S6_OVERLAY_VERSION=MUST_BE_SET_BY_MAKEFILE
ARG BENTHOS_UMH_VERSION=MUST_BE_SET_BY_MAKEFILE
ARG REDPANDA_VERSION=MUST_BE_SET_BY_MAKEFILE
ARG DEBUG=false
ARG RACE=false
ARG PPROF=false
WORKDIR /go/src/github.com/united-manufacturing-hub/united-manufacturing-hub

# Set app version with a default value - can be overridden at build time
ARG APP_VERSION=MUST_BE_SET_BY_MAKEFILE
RUN apk add --no-cache gcc musl-dev
# Copy go.mod first for better caching
COPY ./go.mod ./umh-core/
WORKDIR /go/src/github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core
COPY . .
RUN echo "GOOS: ${GOOS:-$(go env GOOS)}, GOARCH: ${GOARCH:-$(go env GOARCH)}"
RUN echo "Go version: $(go version)"

# Build with conditional flags based on DEBUG, RACE, and PPROF
RUN BUILD_FLAGS=""; \
    LDFLAGS="-X github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants.S6OverlayVersion=${S6_OVERLAY_VERSION} \
    -X github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants.BenthosVersion=${BENTHOS_UMH_VERSION} \
    -X github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants.RedpandaVersion=${REDPANDA_VERSION} \
    -X github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/version.AppVersion=${APP_VERSION}"; \
    \
    if [ "$RACE" = "true" ]; then \
        BUILD_FLAGS="${BUILD_FLAGS} -race"; \
    fi; \
    \
    if [ "$DEBUG" = "true" ]; then \
        BUILD_FLAGS="${BUILD_FLAGS} -gcflags=all=-N -l"; \
    fi; \
    \
    if [ "$PPROF" = "true" ]; then \
        BUILD_FLAGS="${BUILD_FLAGS} -tags pprof -mod=vendor"; \
        LDFLAGS="-s -w ${LDFLAGS}"; \
    elif [ "$DEBUG" != "true" ]; then \
        BUILD_FLAGS="${BUILD_FLAGS} -mod=vendor"; \
        LDFLAGS="-s -w ${LDFLAGS}"; \
    fi; \
    \
    echo "Building with flags: ${BUILD_FLAGS}"; \
    echo "Building with ldflags: ${LDFLAGS}"; \
    CGO_ENABLED=1 go build ${BUILD_FLAGS} -ldflags "${LDFLAGS}" -o /go/bin/umh-core cmd/main.go

# --- Final Stage ---
FROM alpine:${ALPINE_VERSION} as runner
# Copy everything from the downloader stage
COPY --from=downloader / /

# Copy built application
COPY --from=build /go/bin/umh-core /usr/local/bin/umh-core

# Install coreutils (this allows date resolution beyond seconds)
RUN apk add --no-cache coreutils

# Handle DEBUG mode - install Go and Delve when DEBUG=true
# Delve is a debugger for Go (https://github.com/go-delve/delve/)
# We can use it for remote debugging of the UMH Core
ARG DEBUG=false
ARG DLV_VERSION=MUST_BE_SET_BY_MAKEFILE
RUN if [ "$DEBUG" = "true" ]; then \
    apk add --no-cache go && \
    go install github.com/go-delve/delve/cmd/dlv@${DLV_VERSION}; \
    fi

RUN chmod +x /usr/local/bin/umh-core && \
    ls -la /usr/local/bin/umh-core && \
    test -x /usr/local/bin/umh-core || (echo "Binary not executable" && exit 1)

# Copy s6 service configuration
COPY ./s6-base/s6-rc.d /etc/s6-overlay/s6-rc.d/

# Use either standard or debug service based on DEBUG flag
ARG DEBUG=false
RUN if [ "$DEBUG" = "true" ]; then \
    # Use debug service instead of regular service
    cp -f /etc/s6-overlay/s6-rc.d/umh-core-debug/run /etc/s6-overlay/s6-rc.d/umh-core/run && \
    rm -rf /etc/s6-overlay/s6-rc.d/umh-core-debug; \
    chmod +x /etc/s6-overlay/s6-rc.d/umh-core/run; \
    else \
    # Use regular service
    rm -rf /etc/s6-overlay/s6-rc.d/umh-core-debug; \
    chmod +x /etc/s6-overlay/s6-rc.d/umh-core/run; \
    fi

# Set environment variables and volume
# S6_SYNC_DISKS=1: Sync the disks to ensure that the data is persisted, this is needed for the redpanda service
ENV S6_KEEP_ENV=1 \
    S6_BEHAVIOUR_IF_STAGE2_FAILS=2 \
    S6_CMD_WAIT_FOR_SERVICES=1 \
    S6_SERVICES_READYTIME=50 \
    S6_SYNC_DISKS=1 \
    # Important: This needs to be T (ISO 8861) as our time parser expects this format
    S6_LOGGING_SCRIPT="n5 s10000000 T" \
    PATH="/opt/redpanda/bin:$PATH"
VOLUME ["/data"]

ENTRYPOINT ["/init"]
CMD []
