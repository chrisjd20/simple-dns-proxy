#!/bin/bash

# Exit immediately if a command exits with a non-zero status.
set -e

# Define variables
PROJECT_NAME="simple-dns-proxy"
BUILD_DIR="./build"
DOCKER_IMAGE_NAME="simple-dns-proxy-builder"

# Create build directory if it doesn't exist
mkdir -p "${BUILD_DIR}"

# Build the Docker image for compilation
echo "Building Docker image for compilation..."
cat > Dockerfile.build << 'EOF'
FROM golang:1.22-alpine

# Install required tools
RUN apk add --no-cache git zip tar

# Set the working directory
WORKDIR /app

# Copy the Go module files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the source code
COPY . .

# Set up build directory
RUN mkdir -p /app/build/linux-amd64 /app/build/windows-amd64
EOF

docker build -t "${DOCKER_IMAGE_NAME}" -f Dockerfile.build .

# Create a helper build script that will run inside the container
cat > build_binaries.sh << 'EOF'
#!/bin/sh
set -e

# Define variables
PROJECT_NAME="simple-dns-proxy"
BUILD_DIR="./build"

echo "===== Starting cross-compilation ====="

# Create README.txt for distribution
cat > README.txt << 'EOT'
Simple DNS Proxy

USAGE:
  1. Edit config.yaml to match your DNS settings
  2. Run the application with administrator/root privileges
     (required for binding to port 53, which is privileged)

Configuration:
  See config.yaml for all available options

For more information, visit the project repository.
EOT

# Build for Linux (Debian) x86_64
echo "Building for Linux (Debian) x86_64..."
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -buildvcs=false -a -installsuffix cgo -o "${BUILD_DIR}/linux-amd64/${PROJECT_NAME}" .
cp config.yaml "${BUILD_DIR}/linux-amd64/"
cp README.txt "${BUILD_DIR}/linux-amd64/"
tar -czf "${BUILD_DIR}/${PROJECT_NAME}-linux-amd64.tar.gz" -C "${BUILD_DIR}" linux-amd64

# Build for Windows x86_64
echo "Building for Windows (x86_64)..."
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -buildvcs=false -a -installsuffix cgo -o "${BUILD_DIR}/windows-amd64/${PROJECT_NAME}.exe" .
cp config.yaml "${BUILD_DIR}/windows-amd64/"
cp README.txt "${BUILD_DIR}/windows-amd64/"
cd "${BUILD_DIR}" && zip -r "${PROJECT_NAME}-windows-amd64.zip" windows-amd64

echo "===== Cross-compilation complete ====="
echo "Output files:"
echo "  - ${BUILD_DIR}/${PROJECT_NAME}-linux-amd64.tar.gz"
echo "  - ${BUILD_DIR}/${PROJECT_NAME}-windows-amd64.zip"
EOF

chmod +x build_binaries.sh

# Run the Docker container to perform the cross-compilation
echo "Running cross-compilation in Docker container..."
docker run --rm -v "$(pwd):/app" "${DOCKER_IMAGE_NAME}" "/app/build_binaries.sh"

# Cleanup
rm -f Dockerfile.build build_binaries.sh

echo "Cross-compilation completed successfully!"
echo "Binaries are available in the '${BUILD_DIR}' directory:"
echo "  - ${PROJECT_NAME}-linux-amd64.tar.gz"
echo "  - ${PROJECT_NAME}-windows-amd64.zip"