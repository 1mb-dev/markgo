# Build stage
FROM golang:1.26.2-alpine AS builder

# Install ca-certificates for HTTPS requests
RUN apk add --no-cache ca-certificates git tzdata

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a -installsuffix cgo \
    -o markgo \
    ./cmd/markgo

# Final stage - minimal runtime image
FROM scratch

# Copy ca-certificates for HTTPS
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy timezone data
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Copy the binary
COPY --from=builder /app/markgo /markgo

# Copy necessary directories (if they exist)
COPY --from=builder /app/web ./web
COPY --from=builder /app/articles ./articles

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/markgo", "--health-check"] || exit 1

# Set environment variables
ENV ENVIRONMENT=production
ENV PORT=8080
ENV GIN_MODE=release

# Run the binary
ENTRYPOINT ["/markgo"]