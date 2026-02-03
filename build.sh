#!/bin/bash
# Build script for pushoo-chan Go version (Linux/macOS)
# Compiles binaries for multiple platforms
# Run this script from the gover directory

set -e  # Exit on error

echo "========================================"
echo "Building pushoo-chan-gover for multiple platforms"
echo "========================================"

# Create dist directory
mkdir -p dist

# Set Go environment
export CGO_ENABLED=0
export GO111MODULE=on
BUILD_FLAGS='-trimpath -ldflags="-s -w"'

echo ""
echo "[1/5] Building for Linux AMD64..."
GOOS=linux GOARCH=amd64 go build $BUILD_FLAGS -o dist/pushoo-chan-gover-linux-amd64 .
echo "✓ Linux AMD64 build complete"

echo ""
echo "[2/5] Building for Linux ARM64 (ARMv8)..."
GOOS=linux GOARCH=arm64 go build $BUILD_FLAGS -o dist/pushoo-chan-gover-linux-arm64 .
echo "✓ Linux ARM64 build complete"

echo ""
echo "[3/5] Building for Linux ARM (ARMv7)..."
GOOS=linux GOARCH=arm GOARM=7 go build $BUILD_FLAGS -o dist/pushoo-chan-gover-linux-armv7 .
echo "✓ Linux ARMv7 build complete"

echo ""
echo "[4/5] Building for Windows AMD64..."
GOOS=windows GOARCH=amd64 go build $BUILD_FLAGS -o dist/pushoo-chan-gover-windows-amd64.exe .
echo "✓ Windows AMD64 build complete"

echo ""
echo "[5/5] Building for current platform (local test)..."
go build $BUILD_FLAGS -o dist/pushoo-chan-gover-local .
echo "✓ Local build complete"

echo ""
echo "========================================"
echo "Build Summary"
echo "========================================"
ls -lh dist/
echo ""
echo "All builds completed successfully!"
echo "Binaries are located in the 'dist' folder."
echo "========================================"
