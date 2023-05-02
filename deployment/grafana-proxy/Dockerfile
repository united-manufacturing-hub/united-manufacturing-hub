# Copyright 2023 UMH Systems GmbH
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

FROM --platform=$BUILDPLATFORM docker.io/golang:1.20-alpine3.16 AS source
ARG TARGETPLATFORM
ARG BUILDPLATFORM

# Add nonroot user
RUN addgroup -S nonroot \
    && adduser -S nonroot -G nonroot

RUN mkdir /build
# Add build requirements for librdkafka
RUN apk add build-base

WORKDIR /build
ADD ./golang/go.mod /build/go.mod
ADD ./golang/go.sum /build/go.sum
WORKDIR /build

# Only copy relevant packages to docker container
ADD ./golang/cmd/grafana-proxy /build/cmd/grafana-proxy
ADD ./golang/internal /build/internal
ADD ./golang/pkg /build/pkg

RUN go mod tidy

RUN CGO_ENABLED=1 GOOS=linux go test -tags musl,kafka  -v --mod=readonly -buildvcs=false -trimpath ./cmd/grafana-proxy

FROM source AS build
WORKDIR /build
RUN CGO_ENABLED=1 GOOS=linux go build -tags musl,kafka --mod=readonly -buildvcs=false -trimpath -o mainFile ./cmd/grafana-proxy
# Create a list of shared libraries required by the mainFile binary and copy them to the deps directory
RUN mkdir -p /build/deps && ldd mainFile | awk '/=>/ { print $3 }' | xargs -I % sh -c 'echo "Copying: %"; cp % /build/deps/'

FROM source AS build-race
WORKDIR /build
RUN CGO_ENABLED=1 GOOS=linux go build -race -tags musl,kafka --mod=readonly -buildvcs=false -trimpath -o mainFile ./cmd/grafana-proxy
# Create a list of shared libraries required by the mainFile binary and copy them to the deps directory
RUN mkdir -p /build/deps && ldd mainFile | awk '/=>/ { print $3 }' | xargs -I % sh -c 'echo "Copying: %"; cp % /build/deps/'

FROM scratch AS runner-race

COPY --from=build-race /build/mainFile /app/mainFile
COPY --from=build-race /build/deps /lib

COPY --from=source /etc/passwd /etc/passwd
USER nonroot

WORKDIR /app
CMD ["./mainFile"]

FROM scratch AS runner

COPY --from=build /build/mainFile /app/mainFile
COPY --from=build /build/deps /lib

COPY --from=source /etc/passwd /etc/passwd
USER nonroot

WORKDIR /app
CMD ["./mainFile"]
