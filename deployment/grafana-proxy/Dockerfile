FROM golang:alpine as builder

RUN mkdir /build

WORKDIR /build
ADD ./golang/go.mod /build/go.mod
ADD ./golang/go.sum /build/go.sum
WORKDIR /build

# Only copy relevant packages to docker container
ADD ./golang/cmd/grafana-proxy /build/cmd/grafana-proxy




RUN CGO_ENABLED=0 GOOS=linux go test -v --mod=readonly ./cmd/grafana-proxy
RUN CGO_ENABLED=0 GOOS=linux go build -a --mod=readonly -installsuffix cgo -ldflags '-extldflags "-static"' -o mainFile ./cmd/grafana-proxy

FROM scratch
COPY --from=builder /build /app/
WORKDIR /app
CMD ["./mainFile"]
