#!/bin/bash

# Set script to exit immediately if any command fails
set -e

# Load environment variables from .env file if it exists
if [ -f .env ]; then
    echo "Loading environment variables from .env file"
    export $(grep -v '^#' .env | xargs)
else
    echo "Warning: .env file not found"
fi

# Check if environment variables are set
if [ -z "$RELAY_URLS" ] || [ -z "$NOSTR_PVT_KEY" ] || [ -z "$HIVETALK_API_KEY" ] || [ -z "$BASE_URL" ]; then
    echo "Error: Required environment variables must be set"
    echo "Make sure the following variables are set in your .env file or environment:"
    echo "- RELAY_URLS: Comma-separated list of Nostr relay URLs"
    echo "- NOSTR_PVT_KEY: Private key for Nostr bot"
    echo "- HIVETALK_API_KEY: API key for HiveTalk"
    echo "- BASE_URL: Base URL for HiveTalk API"
    echo ""
    echo "Example usage in .env file (no quotes needed):"
    echo "RELAY_URLS=wss://relay1.com,wss://relay2.com"
    echo "NOSTR_PVT_KEY=your-private-key"
    echo "HIVETALK_API_KEY=your-api-key"
    echo "BASE_URL=https://hivetalk.org"
    exit 1
fi

# Display loaded environment variables (masked for security)
echo "Environment variables loaded:"
echo "- RELAY_URLS: ${RELAY_URLS}"
echo "- NOSTR_PVT_KEY: ${NOSTR_PVT_KEY:0:5}... (masked)"
echo "- HIVETALK_API_KEY: ${HIVETALK_API_KEY:0:5}... (masked)"
echo "- BASE_URL: ${BASE_URL}"

# Create log directory if it doesn't exist
mkdir -p logs

# Get current timestamp for log file
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
LOG_FILE="logs/hivetalk_poller_${TIMESTAMP}.log"

echo "Starting HiveTalk poller..."
echo "Logs will be written to ${LOG_FILE}"

# Download dependencies and run the Go program
go mod tidy
go run hivetalk_poller.go 2>&1 | tee "${LOG_FILE}"
