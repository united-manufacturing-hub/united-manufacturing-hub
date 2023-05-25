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

RUN mkdir /build
# Add build requirements for librdkafka
RUN apk add build-base

# Get dependencies
WORKDIR /build
ADD ./golang/go.mod /build/go.mod
ADD ./golang/go.sum /build/go.sum
RUN go mod download

# Only copy relevant packages to docker container
ADD ./golang/cmd/metrics /build/cmd/metrics


RUN go mod tidy

FROM source AS build
WORKDIR /build
RUN CGO_ENABLED=1 GOOS=linux go build -tags musl --mod=readonly -buildvcs=false -trimpath -o mainFile ./cmd/metrics
# Create a list of shared libraries required by the mainFile binary and copy them to the deps directory
RUN mkdir -p /build/deps && ldd mainFile | awk '/=>/ { print $3 }' | xargs -I % sh -c 'echo "Copying: %"; cp % /build/deps/'

FROM source AS build-race
WORKDIR /build
RUN CGO_ENABLED=1 GOOS=linux go build -race -tags musl --mod=readonly -buildvcs=false -trimpath -o mainFile ./cmd/metrics
# Create a list of shared libraries required by the mainFile binary and copy them to the deps directory
RUN mkdir -p /build/deps && ldd mainFile | awk '/=>/ { print $3 }' | xargs -I % sh -c 'echo "Copying: %"; cp % /build/deps/'

FROM alpine AS runner-race
COPY --from=build-race /build/mainFile /app/mainFile
COPY --from=build-race /build/deps /lib

WORKDIR /app
CMD ["./mainFile"]

FROM alpine AS runner

COPY --from=build /build/mainFile /app/mainFile
COPY --from=build /build/deps /lib

WORKDIR /app
CMD ["./mainFile"]
