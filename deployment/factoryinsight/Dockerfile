FROM golang:alpine as builder

RUN mkdir /build

# Only copy relevant packages to docker container
ADD ./golang/cmd/factoryinsight /build/cmd/factoryinsight
ADD ./golang/internal /build/internal
ADD ./golang/pkg /build/pkg
ADD ./golang/test/factoryinsight /build/test/factoryinsight
ADD ./golang/vendor /build/vendor

ADD ./golang/go.mod /build/go.mod
ADD ./golang/go.sum /build/go.sum

WORKDIR /build

RUN CGO_ENABLED=0 GOOS=linux go test ./cmd/factoryinsight
RUN CGO_ENABLED=0 GOOS=linux go build -a --mod=readonly -installsuffix cgo -ldflags '-extldflags "-static"' -o mainFile ./cmd/factoryinsight

FROM scratch
COPY --from=builder /build /app/
WORKDIR /app
CMD ["./mainFile"]
