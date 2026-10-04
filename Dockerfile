FROM golang:1.26.8-alpine3.24@sha256:8ac98ca534ac3f51e1f420a1dd2c15e74c75cfa0f23f3ad27eb5d7236c349a0c AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server

FROM alpine:3.24.1@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b
ENV ENVIRONMENT=production
RUN apk --no-cache add ca-certificates \
    && addgroup -S gympulse \
    && adduser -S -G gympulse gympulse
WORKDIR /app
COPY --from=builder --chown=gympulse:gympulse /app/server .
COPY --chown=gympulse:gympulse migrations/ ./migrations/
USER gympulse
EXPOSE 8080
CMD ["./server"]
