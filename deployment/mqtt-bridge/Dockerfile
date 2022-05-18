FROM golang:alpine as builder

RUN mkdir /build

# Get dependencies
WORKDIR /build
ADD ./golang/go.mod /build/go.mod
ADD ./golang/go.sum /build/go.sum
RUN go mod download

# Only copy relevant packages to docker container
ADD ./golang/cmd/mqtt-bridge /build/cmd/mqtt-bridge
ADD ./golang/internal /build/internal
ADD ./golang/pkg /build/pkg
# ADD ./golang/test/mqtt-bridge /build/test

WORKDIR /build

RUN CGO_ENABLED=0 GOOS=linux go build -a --mod=readonly -installsuffix cgo -ldflags "-X 'main.buildtime=$(date -u '+%Y-%m-%d %H:%M:%S')' -extldflags '-static'" -o mainFile ./cmd/mqtt-bridge

FROM scratch
COPY --from=builder /build /app/
WORKDIR /app
CMD ["./mainFile"]
