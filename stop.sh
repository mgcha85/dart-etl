#!/bin/bash

if [ -f server.pid ]; then
    PID=$(cat server.pid)
    echo "Stopping server (PID: $PID)..."
    kill $PID
    rm server.pid
    echo "Server stopped."
else
    echo "server.pid file not found. Is the server running?"
fi
