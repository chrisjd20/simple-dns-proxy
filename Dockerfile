FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copy go.mod file first (without go.sum) to leverage Docker caching
COPY go.mod ./

# Copy the rest of the source code
COPY . .

# Run go mod tidy to ensure go.mod and go.sum are accurate and all dependencies are accounted for.
# This will also download dependencies if they are not already present from go.sum.
RUN go mod tidy

# As an extra step, explicitly download dependencies.
# go mod tidy might have already done this, but this ensures it.
RUN go mod download

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o simple-dns-proxy .

# Final stage
FROM alpine:latest

WORKDIR /app

# Install ca-certificates for HTTPS calls if needed by the application (e.g. external services)
# and tini for proper signal handling and zombie reaping.
RUN apk --no-cache add ca-certificates tini

# Copy the config file and the built binary from the builder stage
COPY --from=builder /app/simple-dns-proxy .

# Create a non-root user and group
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

# Create config directory and set permissions
RUN mkdir -p /app/config && chown -R appuser:appgroup /app/config && chmod -R 755 /app/config

# Switch to the non-root user
USER appuser

# Expose UDP and TCP port 53 for DNS
EXPOSE 53/udp
EXPOSE 53/tcp

# Set tini as the entrypoint
ENTRYPOINT ["/sbin/tini", "--"]

# Run the application
CMD ["./simple-dns-proxy"]
