# Build stage
FROM golang:1.26.6-alpine AS builder

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

# Output architecture. Defaults to amd64 so the legacy builder keeps producing exactly
# what it always has; buildx populates TARGETARCH itself from --platform, and an explicit
# --build-arg overrides either. Never hardcode the arch again — an arm64 host silently
# building amd64 is a broken image nobody notices until it will not start.
ARG TARGETARCH=amd64

# CGO_ENABLED=0 already produces a static binary, so -extldflags "-static" (no external
# linker runs) and -installsuffix cgo (a no-op since Go 1.10) did nothing, while -a forced
# a full standard-library rebuild on every build.
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build \
    -trimpath \
    -ldflags='-w -s' \
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

# Health check. Exec form with no shell operators — a trailing `|| exit 1` makes Docker
# parse this as CMD-SHELL, and the runtime stage is FROM scratch with no /bin/sh, so the
# probe fails to launch at all. The exit code of the binary is the verdict on its own.
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/markgo", "--health-check"]

# Set environment variables
ENV ENVIRONMENT=production
ENV PORT=8080
ENV GIN_MODE=release

# Run the binary
ENTRYPOINT ["/markgo"]