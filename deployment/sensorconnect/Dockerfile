FROM golang:alpine as builder

RUN mkdir /build
# Add build requirements for librdkafka
RUN apk add build-base

# Get dependencies
WORKDIR /build
ADD ./golang/go.mod /build/go.mod
ADD ./golang/go.sum /build/go.sum
RUN go mod download

# Only copy relevant packages to docker container
ADD ./golang/cmd/sensorconnect /build/cmd/sensorconnect
ADD ./golang/internal /build/internal
ADD ./golang/pkg /build/pkg
# ADD ./golang/test/sensorconnect /build/test

WORKDIR /build

#RUN CGO_ENABLED=0 GOOS=linux go test --mod=readonly ./cmd/sensorconnect
RUN GOOS=linux go build -tags musl -a --mod=readonly -installsuffix cgo -ldflags "-X 'main.buildtime=$(date -u '+%Y-%m-%d %H:%M:%S')' -extldflags '-static'" -o mainFile ./cmd/sensorconnect

FROM alpine:latest as certs
RUN apk --update add ca-certificates

FROM scratch
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /build /app/
WORKDIR /app
CMD ["./mainFile"]
