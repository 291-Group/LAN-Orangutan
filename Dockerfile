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

# Version details, supplied by the release workflow. Without these the image
# reports whatever it can work out from the repository, which for a build
# context with no .git means "unknown".
ARG VERSION=""
ARG COMMIT=""
ARG BUILD_DATE=""

# Build binary
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w \
      -X 'github.com/291-Group/LAN-Orangutan/internal/cli.Version=${VERSION}' \
      -X 'github.com/291-Group/LAN-Orangutan/internal/cli.Commit=${COMMIT}' \
      -X 'github.com/291-Group/LAN-Orangutan/internal/cli.BuildDate=${BUILD_DATE}'" \
    -o orangutan ./cmd/orangutan

# Runtime stage
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache nmap nmap-scripts ca-certificates tzdata


# Create directories
RUN mkdir -p /etc/lan-orangutan /var/lib/lan-orangutan

# Copy binary from builder
COPY --from=builder /app/orangutan /usr/local/bin/orangutan
COPY config.example.ini /etc/lan-orangutan/config.ini

# Set permissions
RUN chmod +x /usr/local/bin/orangutan

# The app resolves its data directory from the running user's home, which for
# this non-root user would not be the volume mounted by docker-compose. Pin it
# so the mount and the app agree; otherwise data silently vanishes on restart.
ENV ORANGUTAN_DATA_DIR=/var/lib/lan-orangutan

# Expose port
EXPOSE 291

# Runs as root inside the container.
#
# This is the same choice the systemd service makes, and for the same reason:
# nmap needs raw sockets to read the ARP table, which is what supplies MAC
# addresses and manufacturer names, and port 291 is privileged. Capabilities
# granted with cap_add land in the permitted set but never become effective for
# a process that starts as a non-root user, so running unprivileged here meant
# the container could neither bind its port nor identify a single device.
#
# The container itself remains the isolation boundary.

# Health check.
#
# Targets a static asset over 127.0.0.1: it is served without authentication and
# returns 200 in every state (before setup, after a password is set, signed out),
# so the check reflects "is the server up" rather than "is it unlocked". Uses
# 127.0.0.1 rather than localhost because the server binds IPv4 and localhost can
# resolve to IPv6 ::1.
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://127.0.0.1:291/static/orangutan.svg || exit 1

ENTRYPOINT ["orangutan"]
CMD ["serve"]
