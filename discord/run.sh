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
if [ -z "$RELAY_URL" ] || [ -z "$DISCORD_WEBHOOK" ]; then
    echo "Error: RELAY_URL and DISCORD_WEBHOOK environment variables must be set"
    echo "Example usage:"
    echo "export RELAY_URL='wss://your-relay.com'"
    echo "export DISCORD_WEBHOOK='https://discord.com/api/webhooks/...'"
    exit 1
fi

# Display loaded environment variables (masked for security)
echo "Environment variables loaded:"
echo "- RELAY_URL: ${RELAY_URL}"
echo "- DISCORD_WEBHOOK: ${DISCORD_WEBHOOK:0:5}... (masked)"

# Download dependencies and run the Go program
go mod tidy
go run nostr_listener.go
