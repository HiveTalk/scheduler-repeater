#!/bin/bash

# Check if environment variables are set
if [ -z "$RELAY_URL" ] || [ -z "$DISCORD_WEBHOOK" ]; then
    echo "Error: RELAY_URL and DISCORD_WEBHOOK environment variables must be set"
    echo "Example usage:"
    echo "export RELAY_URL='wss://your-relay.com'"
    echo "export DISCORD_WEBHOOK='https://discord.com/api/webhooks/...'"
    exit 1
fi

# Download dependencies and run the Go program
go mod tidy
go run nostr_listener.go
