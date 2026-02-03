#!/bin/bash
# Docker update script for pushoo-chan-gover
# This script will:
# 1. Stop and remove existing container
# 2. Remove old image
# 3. Build new image from dist binaries
# 4. Start new container

set -e  # Exit on error

CONTAINER_NAME="pushoo-chan-gover"
IMAGE_NAME="pushoo-chan-gover:latest"

echo "========================================"
echo "Docker Update Script for pushoo-chan-gover"
echo "========================================"

# Check if dist folder exists and has the required binary
if [ ! -f "dist/pushoo-chan-gover-linux-amd64" ]; then
    echo "ERROR: Binary not found at dist/pushoo-chan-gover-linux-amd64"
    echo "Please run build.sh first to compile the binaries."
    exit 1
fi

echo ""
echo "[1/5] Stopping existing container (if running)..."
if docker ps -q -f name=$CONTAINER_NAME | grep -q .; then
    docker stop $CONTAINER_NAME
    echo "✓ Container stopped"
else
    echo "ℹ Container not running"
fi

echo ""
echo "[2/5] Removing existing container (if exists)..."
if docker ps -a -q -f name=$CONTAINER_NAME | grep -q .; then
    docker rm $CONTAINER_NAME
    echo "✓ Container removed"
else
    echo "ℹ Container does not exist"
fi

echo ""
echo "[3/5] Removing old image (if exists)..."
if docker images -q $IMAGE_NAME | grep -q .; then
    docker rmi $IMAGE_NAME
    echo "✓ Image removed"
else
    echo "ℹ Image does not exist"
fi

echo ""
echo "[4/5] Building new Docker image..."
docker-compose build --no-cache
echo "✓ Image built successfully"

echo ""
echo "[5/5] Starting new container..."
docker-compose up -d
echo "✓ Container started successfully"

echo ""
echo "========================================"
echo "Update Complete!"
echo "========================================"
echo "Container name: $CONTAINER_NAME"
echo "Image: $IMAGE_NAME"
echo "Port: 8084"
echo ""
echo "Check container status:"
echo "  docker ps | grep $CONTAINER_NAME"
echo ""
echo "View logs:"
echo "  docker logs -f $CONTAINER_NAME"
echo ""
echo "Access web interface:"
echo "  http://localhost:8084"
echo "========================================"
