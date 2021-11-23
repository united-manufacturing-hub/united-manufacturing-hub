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
ADD ./golang/cmd/kafka-to-blob /build/cmd/kafka-to-blob
ADD ./golang/internal /build/internal
ADD ./golang/pkg /build/pkg
# ADD ./golang/test/kafka-to-blob /build/test

WORKDIR /build

RUN GOOS=linux go build -tags musl -a --mod=readonly -installsuffix cgo -ldflags "-X 'main.buildtime=$(date -u '+%Y-%m-%d %H:%M:%S')' -extldflags '-static'" -o mainFile ./cmd/kafka-to-blob

FROM scratch
COPY --from=builder /build /app/
WORKDIR /app
CMD ["./mainFile"]