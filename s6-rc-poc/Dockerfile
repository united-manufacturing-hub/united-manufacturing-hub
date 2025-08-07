FROM golang:1.24-alpine as builder
WORKDIR /build
COPY go.mod .
COPY cmd .
RUN CGO_ENABLED=1 go build -race -ldflags "-s -w" -o /s6-rc-poc

FROM alpine as runner
COPY --from=builder /s6-rc-poc /s6-rc-poc

ENTRYPOINT ["/s6-rc-poc"]
