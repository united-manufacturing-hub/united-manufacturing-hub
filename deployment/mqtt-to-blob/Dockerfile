FROM golang:alpine as builder

RUN mkdir /build

# Get dependencies
WORKDIR /build
ADD ./golang/go.mod /build/go.mod
ADD ./golang/go.sum /build/go.sum
RUN go mod download

# Only copy relevant packages to docker container
ADD ./golang/cmd/mqtt-to-blob /build/cmd/mqtt-to-blob
ADD ./golang/internal /build/internal
ADD ./golang/pkg /build/pkg
# ADD ./golang/test/mqtt-to-blob /build/test

WORKDIR /build

RUN GOOS=linux go build -tags musl -a --mod=readonly -installsuffix cgo -ldflags "-X 'main.buildtime=$(date -u '+%Y-%m-%d %H:%M:%S')' -extldflags '-static'" -o mainFile ./cmd/mqtt-to-blob

FROM scratch
COPY --from=builder /build /app/
WORKDIR /app
CMD ["./mainFile"]
