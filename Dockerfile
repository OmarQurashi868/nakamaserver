# Build stage
FROM golang:1.26-alpine AS builder

WORKDIR /app

# Copy dependency files first for caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o nakamaserver ./cmd/nakamaserver

# Final run stage
FROM alpine:3.21

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user
RUN addgroup -S nakama && adduser -S -G nakama nakama

# Create storage directories and set ownership
RUN mkdir -p /data/nakama/games /data/nakama/modpacks && \
    chown -R nakama:nakama /data/nakama

WORKDIR /app
COPY --from=builder --chown=nakama:nakama /app/nakamaserver .

USER nakama

EXPOSE 8080

ENTRYPOINT ["./nakamaserver"]
