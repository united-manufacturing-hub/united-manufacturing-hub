FROM golang:1.19-alpine3.16 as builder

RUN mkdir /build
# Add build requirements for librdkafka
RUN apk add build-base

# Get dependencies
WORKDIR /build
ADD ./golang/go.mod /build/go.mod
ADD ./golang/go.sum /build/go.sum
RUN go mod download

# Only copy relevant packages to docker container
ADD ./golang/cmd/factoryinsight /build/cmd/factoryinsight
ADD ./golang/internal /build/internal
ADD ./golang/pkg /build/pkg
ADD ./golang/test/factoryinsight /build/test/factoryinsight

RUN GOOS=linux go build -tags musl,kafka -a --mod=readonly -installsuffix cgo -ldflags "-X 'main.buildtime=$(date -u '+%Y-%m-%d %H:%M:%S')' -extldflags '-static'" -o mainFile ./cmd/factoryinsight
RUN GOOS=linux go test -tags musl,kafka -v --mod=readonly ./cmd/factoryinsight

FROM scratch
COPY --from=builder /build /app/
WORKDIR /app
CMD ["./mainFile"]
