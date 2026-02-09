FROM golang:1.24 AS builder
WORKDIR /app
COPY go.mod .
COPY main.go .
RUN CGO_ENABLED=0 go build -o chaos-proxy .

FROM alpine:3.21
COPY --from=builder /app/chaos-proxy /usr/local/bin/chaos-proxy
ENTRYPOINT ["chaos-proxy"]
