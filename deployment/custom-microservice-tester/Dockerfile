FROM golang:1.19-alpine3.16 as builder

RUN mkdir /build

# Get dependencies
WORKDIR /build
ADD ./golang/go.mod /build/go.mod
ADD ./golang/go.sum /build/go.sum
RUN go mod download

# Only copy relevant packages to docker container
ADD ./golang/cmd/custom-microservice-tester /build/cmd/custom-microservice-tester

RUN CGO_ENABLED=0 GOOS=linux go build -a --mod=readonly -installsuffix cgo -ldflags "-X 'main.buildtime=$(date -u '+%Y-%m-%d %H:%M:%S')' -extldflags '-static'" -o mainFile ./cmd/custom-microservice-tester

FROM scratch
COPY --from=builder /build /app/
WORKDIR /app
CMD ["./mainFile"]
