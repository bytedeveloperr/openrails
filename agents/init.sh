#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_ROOT"

cleanup() {
    echo ""
    echo "Shutting down docker compose..."
    docker compose down
    echo "Cleanup complete."
}

trap cleanup EXIT INT TERM

echo "Starting docker compose infrastructure..."
docker compose up -d

echo "Waiting for migrations to complete..."
while ! docker compose ps -a billing-migrate --format '{{.State}}' 2>/dev/null | grep -q "exited"; do
    sleep 1
done

# Check if migrations succeeded
EXIT_CODE=$(docker compose ps -a billing-migrate --format '{{.ExitCode}}')
if [ "$EXIT_CODE" != "0" ]; then
    echo "Migrations failed with exit code $EXIT_CODE!"
    docker compose logs billing-migrate
    exit 1
fi

echo "Migrations complete."
echo "Starting doujins-billing server locally..."

# Set environment variables to connect to docker compose services
export DB_URL="postgres://admin:admin_password@localhost:5432/doujins_db?sslmode=disable"
export REDIS_ADDR="localhost:6382"
export CLICKHOUSE_HTTP_ADDR="http://localhost:8123"
export CLICKHOUSE_CLIENT_ADDR="localhost:9002"
export CLICKHOUSE_DB="analytics"
export CLICKHOUSE_USER="analytics_user"
export CLICKHOUSE_PASSWORD="analytics_password"

go run . server --start-workers
