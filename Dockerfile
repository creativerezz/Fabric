# Use official golang image as builder
FROM golang:1.24.2-alpine AS builder

# Set working directory
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o fabric

# Use scratch as final base image
FROM alpine:latest

# Copy the binary from builder
COPY --from=builder /app/fabric /fabric

# Copy patterns directory into the Fabric config folder
# This ensures patterns are available in the expected DB location
RUN mkdir -p /root/.config/fabric/patterns
COPY patterns /root/.config/fabric/patterns

# Reset config directory (keeps it empty for environment-based config)
RUN rm -rf /root/.config/fabric && \
    mkdir -p /root/.config/fabric
# The application will load configuration from environment variables; a .env file is optional

# Add debug commands
RUN ls -la /root/.config/fabric/

## Port exposure and startup configured via PORT environment variable below
# Expose port and configure runtime to bind on the PORT environment variable
ENV PORT=8080
EXPOSE 8080
# Run the Fabric REST API, binding to $PORT at runtime (default 8080)
ENTRYPOINT ["sh", "-c", "/fabric --serve --address :${PORT:-8080}"]
