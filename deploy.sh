#!/bin/bash
# DART ETL - Podman Deployment Script

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}=== DART ETL Podman Deployment ===${NC}"

# Check if podman is installed
if ! command -v podman &> /dev/null; then
    echo -e "${RED}Error: podman is not installed${NC}"
    echo "Install podman: sudo apt install podman"
    exit 1
fi

# Check if podman-compose is installed
if ! command -v podman-compose &> /dev/null; then
    echo -e "${YELLOW}Warning: podman-compose is not installed${NC}"
    echo "Installing podman-compose via pip..."
    pip install podman-compose
fi

# Show versions
echo -e "${GREEN}Podman version:${NC}"
podman --version

echo -e "${GREEN}Podman-compose version:${NC}"
podman-compose --version

# Parse command line arguments
ACTION="${1:-up}"

case "$ACTION" in
    build)
        echo -e "${GREEN}Building container image...${NC}"
        podman-compose build
        ;;
    up)
        echo -e "${GREEN}Building and starting containers...${NC}"
        podman-compose up -d --build
        echo -e "${GREEN}Container status:${NC}"
        podman ps --filter "name=dart-etl"
        ;;
    down)
        echo -e "${YELLOW}Stopping containers...${NC}"
        podman-compose down
        ;;
    logs)
        echo -e "${GREEN}Showing logs...${NC}"
        podman-compose logs -f dart-etl
        ;;
    restart)
        echo -e "${YELLOW}Restarting containers...${NC}"
        podman-compose restart
        ;;
    status)
        echo -e "${GREEN}Container status:${NC}"
        podman ps --filter "name=dart-etl"
        ;;
    *)
        echo "Usage: $0 {build|up|down|logs|restart|status}"
        echo ""
        echo "Commands:"
        echo "  build   - Build container image only"
        echo "  up      - Build and start containers (default)"
        echo "  down    - Stop and remove containers"
        echo "  logs    - Show container logs (follow mode)"
        echo "  restart - Restart containers"
        echo "  status  - Show container status"
        exit 1
        ;;
esac

echo -e "${GREEN}Done!${NC}"
