#!/bin/bash

# Ensure log directory exists
mkdir -p logs

echo "Building DART ETL Server..."
go build -o dart-etl-server cmd/server/main.go
if [ $? -ne 0 ]; then
    echo "Build failed. Exiting."
    exit 1
fi

echo "Starting DART ETL Server..."
nohup ./dart-etl-server > logs/server.log 2>&1 &

echo $! > server.pid
echo "Server started with PID $(cat server.pid)"
echo "Logs are being written to logs/server.log"
