#!/usr/bin/env bash
# Basic functional verification for WeKnora application
# This script runs inside the running container.
# It performs a health check and a simple API call.

set -e

HOST="http://localhost:8080"

echo "=== Healthcheck ==="
curl -fsSL "$HOST/health" || { echo "Healthcheck failed"; exit 1; }

echo "Healthcheck passed"

# Attempt to list knowledge bases (example endpoint). Adjust if actual API differs.
echo "=== List Knowledge Bases ==="
curl -fsSL "$HOST/api/kb" || { echo "Failed to list knowledge bases"; exit 1; }

echo "Functional test completed successfully"
exit 0
