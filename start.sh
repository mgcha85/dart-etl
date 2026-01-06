#!/bin/bash

echo "Building and starting DART ETL container with Podman..."

# Build the image
podman-compose build

# Start the container in detached mode
podman-compose up -d

echo "Container started. Use 'podman logs dart-etl-app -f' to view logs."
