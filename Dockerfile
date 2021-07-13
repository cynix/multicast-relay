FROM docker.io/library/golang:alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /build
COPY . .
RUN go build -v -trimpath -ldflags '-s -w'

FROM docker.io/library/alpine:latest

COPY --from=builder /build/multicast-relay /usr/sbin/multicast-relay

ENTRYPOINT ["/usr/sbin/multicast-relay"]
