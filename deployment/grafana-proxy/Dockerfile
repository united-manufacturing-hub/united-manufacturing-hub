FROM golang:alpine as builder

RUN mkdir /build

WORKDIR /build
ADD ./golang/go.mod /build/go.mod
ADD ./golang/go.sum /build/go.sum
WORKDIR /build

# Only copy relevant packages to docker container
ADD ./golang/cmd/grafanaproxy /build/cmd/grafanaproxy
ADD ./golang/internal /build/internal
ADD ./golang/pkg /build/pkg


RUN CGO_ENABLED=0 GOOS=linux go test -v --mod=readonly ./cmd/grafanaproxy
RUN CGO_ENABLED=0 GOOS=linux go build -a --mod=readonly -installsuffix cgo -ldflags '-extldflags "-static"' -o mainFile ./cmd/grafanaproxy

FROM scratch
COPY --from=builder /build /app/
WORKDIR /app
CMD ["./mainFile"]
