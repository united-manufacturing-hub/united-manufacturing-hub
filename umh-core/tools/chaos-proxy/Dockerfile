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

FROM golang:1.24 AS builder
WORKDIR /app
COPY go.mod .
COPY main.go .
RUN CGO_ENABLED=0 go build -o chaos-proxy .

FROM alpine:3.21
COPY --from=builder /app/chaos-proxy /usr/local/bin/chaos-proxy
ENTRYPOINT ["chaos-proxy"]
