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

FROM docker.io/golang:1.19-alpine3.16 as builder

RUN mkdir /build
# Add build requirements for librdkafka
RUN apk add build-base gcc musl-dev

# Get dependencies
WORKDIR /build
ADD ./golang/go.mod /build/go.mod
ADD ./golang/go.sum /build/go.sum
RUN go mod download

# Only copy relevant packages to docker container
ADD ./golang/cmd/kafka-state-detector /build/cmd/kafka-state-detector
ADD ./golang/internal /build/internal
ADD ./golang/pkg /build/pkg
# ADD ./golang/test/kafka-state-detector /build/test

WORKDIR /build

RUN go mod tidy
RUN CGO_ENABLED=1 GOOS=linux go build -tags musl,kafka --mod=readonly -buildvcs=false -trimpath -o mainFile ./cmd/kafka-state-detector


# Create a list of shared libraries required by the mainFile binary and copy them to the deps directory
RUN mkdir -p /build/deps && ldd mainFile | awk '/=>/ { print $3 }' | xargs -I % sh -c 'echo "Copying: %"; cp % /build/deps/'

FROM scratch
COPY --from=builder /build/mainFile /app/mainFile
COPY --from=builder /build/deps /lib

WORKDIR /app
CMD ["./mainFile"]
