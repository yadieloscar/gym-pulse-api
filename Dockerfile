FROM golang:1.26.11-alpine3.24@sha256:ae52b5bc6ed045a9ed26ab03dc19327a802a9cc9aa8b27ed57f5c6b60579b0bb AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# VERSION is the source commit; the deploy workflow passes it so /health can
# report which build is serving.
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
      -ldflags "-X github.com/gym-pulse/gym-pulse-api/internal/buildinfo.Version=${VERSION}" \
      -o /out/ ./cmd/server ./cmd/migrate

FROM alpine:3.24.2@sha256:d56c381f961d307a21b3ca004cf1e3910f106644aefb1f43e654c8a56c4fd395
ENV ENVIRONMENT=production
RUN apk --no-cache upgrade \
    && apk --no-cache add ca-certificates \
    && addgroup -S gympulse \
    && adduser -S -G gympulse gympulse
WORKDIR /app
COPY --from=builder --chown=gympulse:gympulse /out/server /out/migrate ./
COPY --chown=gympulse:gympulse migrations/ ./migrations/
USER gympulse
EXPOSE 8080
CMD ["./server"]
