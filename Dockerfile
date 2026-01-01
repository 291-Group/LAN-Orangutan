# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o orangutan ./cmd/orangutan

# Runtime stage
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache nmap nmap-scripts ca-certificates tzdata

# Create non-root user
RUN addgroup -S orangutan && adduser -S orangutan -G orangutan

# Create directories
RUN mkdir -p /etc/lan-orangutan /var/lib/lan-orangutan \
    && chown -R orangutan:orangutan /var/lib/lan-orangutan

# Copy binary from builder
COPY --from=builder /app/orangutan /usr/local/bin/orangutan
COPY config.example.ini /etc/lan-orangutan/config.ini

# Set permissions
RUN chmod +x /usr/local/bin/orangutan

# Expose port
EXPOSE 291

# Run as non-root (but nmap needs NET_RAW capability)
USER orangutan

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:291/api/status || exit 1

ENTRYPOINT ["orangutan"]
CMD ["serve"]
