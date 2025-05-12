#!/bin/bash

# Exit immediately if a command exits with a non-zero status.
set -e

# Print info message
print_info() {
    echo -e "\033[1;34m[INFO]\033[0m $1"
}

# Print success message
print_success() {
    echo -e "\033[1;32m[SUCCESS]\033[0m $1"
}

# Print error message
print_error() {
    echo -e "\033[1;31m[ERROR]\033[0m $1"
}

# Build directory
BUILD_DIR="./build"
APP_NAME="simple-dns-proxy"

# Create build directory if it doesn't exist
mkdir -p "$BUILD_DIR"

print_info "Starting cross-compilation for multiple platforms"

# Ensure dependencies are properly downloaded
print_info "Ensuring dependencies are up to date..."
go mod tidy

# Function to build for a specific OS and architecture
build_for() {
    local OS=$1
    local ARCH=$2
    local EXTENSION=$3
    local OUTPUT_DIR="$BUILD_DIR/$OS-$ARCH"
    local FILE_NAME="${APP_NAME}${EXTENSION}"
    
    print_info "Building for $OS/$ARCH..."
    
    mkdir -p "$OUTPUT_DIR"
    
    # Set environment variables for cross-compilation
    GOOS=$OS GOARCH=$ARCH CGO_ENABLED=0 go build -a -ldflags="-s -w" -o "$OUTPUT_DIR/$FILE_NAME" .
    
    if [ $? -eq 0 ]; then
        print_success "Successfully built $APP_NAME for $OS/$ARCH"
        
        # Copy config.yaml template to the build directory
        cp config.yaml "$OUTPUT_DIR/"
        
        # Create a README.txt with basic instructions
        cat > "$OUTPUT_DIR/README.txt" << EOL
Simple DNS Proxy

This is a standalone DNS proxy that handles A records based on entries in config.yaml
and relays other queries to a fallback DNS.

Usage:
1. Edit config.yaml to configure your DNS records and server settings
2. Run the executable: ./${FILE_NAME}

The DNS proxy will start and handle DNS queries according to your configuration.
EOL
    else
        print_error "Failed to build for $OS/$ARCH"
    fi
}

# Build for Linux (Debian) x86_64
build_for "linux" "amd64" ""

# Build for Windows x86_64
build_for "windows" "amd64" ".exe"

print_info "Creating distribution archives..."

# Create distribution archives
cd "$BUILD_DIR"

# Create tar.gz for Linux
tar -czf "${APP_NAME}-linux-amd64.tar.gz" linux-amd64/

# Create zip for Windows
if command -v zip >/dev/null 2>&1; then
    zip -r "${APP_NAME}-windows-amd64.zip" windows-amd64/
else
    print_error "zip command not found; Windows archive not created"
    print_info "You can install it with: sudo apt-get install zip"
fi

cd - >/dev/null

print_success "Cross-compilation completed successfully!"
print_info "Binaries are available in the $BUILD_DIR directory:"
print_info "  - Linux (Debian) x86_64: $BUILD_DIR/linux-amd64/$APP_NAME"
print_info "  - Windows x86_64: $BUILD_DIR/windows-amd64/${APP_NAME}.exe"
print_info "Archives are available in the $BUILD_DIR directory:"
print_info "  - Linux (Debian) x86_64: $BUILD_DIR/${APP_NAME}-linux-amd64.tar.gz"
print_info "  - Windows x86_64: $BUILD_DIR/${APP_NAME}-windows-amd64.zip (if zip is installed)"
