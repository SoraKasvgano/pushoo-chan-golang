# Multi-stage build for pushoo-chan Go version
FROM alpine:latest

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates tzdata

# Set timezone to Asia/Shanghai (or your preferred timezone)
ENV TZ=Asia/Shanghai

# Create app directory and user
RUN mkdir -p /app/data && \
    addgroup -g 1000 pushoo && \
    adduser -D -u 1000 -G pushoo pushoo && \
    chown -R pushoo:pushoo /app

WORKDIR /app

# Copy pre-built binary from dist folder
# The binary should be built for linux/amd64 or the target architecture
COPY --chown=pushoo:pushoo dist/pushoo-chan-gover-linux-amd64 /app/pushoo-chan-gover
RUN chmod +x /app/pushoo-chan-gover

# Switch to non-root user
USER pushoo

# Volume for configuration and data
# The config.yaml will be automatically created in /app/data if it doesn't exist
VOLUME /app/data

# Expose port
EXPOSE 8084

# Run the application
# Config file will be created at /app/data/config.yaml on first run
CMD ["/app/pushoo-chan-gover", "-config", "/app/data/config.yaml", "-addr", ":8084"]
