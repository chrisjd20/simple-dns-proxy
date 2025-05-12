#!/bin/bash

# Exit immediately if a command exits with a non-zero status.
set -e

# Define variables
IMAGE_NAME="simple-dns-proxy"
CONTAINER_NAME="simple-dns-proxy-container"
CONFIG_FILE_HOST_PATH="$(pwd)/config.yaml"
CONFIG_FILE_CONTAINER_PATH="/app/config/config.yaml"

# Build the Docker image
echo "Building Docker image: $IMAGE_NAME..."
docker build -t $IMAGE_NAME .

echo "Docker image built successfully."

# Stop and remove existing container if it exists
if [ "$(docker ps -q -f name=$CONTAINER_NAME)" ]; then
    echo "Stopping existing container: $CONTAINER_NAME..."
    docker stop $CONTAINER_NAME
    echo "Removing existing container: $CONTAINER_NAME..."
    docker rm $CONTAINER_NAME
fi

# Run the Docker container
echo "Running Docker container: $CONTAINER_NAME..."
echo "Host config file: $CONFIG_FILE_HOST_PATH"
echo "Container config file: $CONFIG_FILE_CONTAINER_PATH"

# Ensure the host config file exists
if [ ! -f "$CONFIG_FILE_HOST_PATH" ]; then
    echo "Error: Configuration file not found at $CONFIG_FILE_HOST_PATH" >&2
    echo "Please create it with your DNS records." >&2
    exit 1
fi

docker run --rm -it \
    --name $CONTAINER_NAME \
    -p 53:53/udp \
    -p 53:53/tcp \
    -v "$CONFIG_FILE_HOST_PATH":"$CONFIG_FILE_CONTAINER_PATH":ro \
    $IMAGE_NAME

echo "Container $CONTAINER_NAME started."
echo "DNS proxy should be available on UDP port 53."
echo "To see logs: docker logs -f $CONTAINER_NAME"
echo "To stop: docker stop $CONTAINER_NAME"
