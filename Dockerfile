FROM golang:1.26-alpine AS builder
COPY core/ /build/core/
COPY cache-redis/ /build/cache-redis/
WORKDIR /build/cache-redis
RUN go mod download
RUN CGO_ENABLED=0 go build -o /cache-redis ./cmd/module
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /cache-redis /
ENTRYPOINT ["/cache-redis"]
