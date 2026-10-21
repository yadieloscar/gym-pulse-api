FROM golang:1.26.10-alpine3.24@sha256:95086244f14745c3afe829d2d46d6d34739dc6850ebef3994b2c1fa6a6f6bbb9 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server

FROM alpine:3.24.2@sha256:d56c381f961d307a21b3ca004cf1e3910f106644aefb1f43e654c8a56c4fd395
ENV ENVIRONMENT=production
RUN apk --no-cache upgrade \
    && apk --no-cache add ca-certificates \
    && addgroup -S gympulse \
    && adduser -S -G gympulse gympulse
WORKDIR /app
COPY --from=builder --chown=gympulse:gympulse /app/server .
COPY --chown=gympulse:gympulse migrations/ ./migrations/
USER gympulse
EXPOSE 8080
CMD ["./server"]
