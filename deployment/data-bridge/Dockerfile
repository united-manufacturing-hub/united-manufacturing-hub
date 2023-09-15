FROM --platform=$BUILDPLATFORM docker.io/golang:1.21-alpine3.18 AS source
ARG TARGETPLATFORM
ARG BUILDPLATFORM

# Add nonroot user
RUN addgroup -S nonroot \
    && adduser -S nonroot -G nonroot

RUN mkdir /build
# Add build requirements for librdkafka
RUN apk add build-base

# Get dependencies
WORKDIR /build
ADD ./golang/go.mod /build/go.mod
ADD ./golang/go.sum /build/go.sum
RUN go mod download

# Only copy relevant packages to docker container
ADD ./golang/cmd/data-bridge /build/cmd/data-bridge
ADD ./golang/internal /build/internal
ADD ./golang/pkg /build/pkg


RUN go mod tidy

FROM source AS build
WORKDIR /build
RUN CGO_ENABLED=1 GOOS=linux go build -tags musl,kafka --mod=readonly -buildvcs=false -trimpath -o mainFile ./cmd/data-bridge
# Create a list of shared libraries required by the mainFile binary and copy them to the deps directory
RUN mkdir -p /build/deps && ldd mainFile | awk '/=>/ { print $3 }' | xargs -I % sh -c 'echo "Copying: %"; cp % /build/deps/'

FROM source AS build-race
WORKDIR /build
RUN CGO_ENABLED=1 GOOS=linux go build -race -tags musl,kafka --mod=readonly -buildvcs=false -trimpath -o mainFile ./cmd/data-bridge
# Create a list of shared libraries required by the mainFile binary and copy them to the deps directory
RUN mkdir -p /build/deps && ldd mainFile | awk '/=>/ { print $3 }' | xargs -I % sh -c 'echo "Copying: %"; cp % /build/deps/'

FROM scratch AS runner-race

COPY --from=build-race /build/mainFile /app/mainFile
COPY --from=build-race /build/deps /lib

COPY --from=source /etc/passwd /etc/passwd
USER nonroot

WORKDIR /app
CMD ["./mainFile"]

FROM scratch AS runner

COPY --from=build /build/mainFile /app/mainFile
COPY --from=build /build/deps /lib

COPY --from=source /etc/passwd /etc/passwd
USER nonroot

WORKDIR /app
CMD ["./mainFile"]
