# Hivetalk Scheduler (Go Implementation)

This is a Go implementation of the Hivetalk scheduler that manages Nostr events for scheduled meetings.

## Dependencies

- `github.com/jackc/pgx/v5` - PostgreSQL driver for Go
- `github.com/nbd-wtf/go-nostr` - Go implementation of the Nostr protocol

## Environment Variables

The following environment variables need to be set:

- `DATABASE_URL` - The Supabase PostgreSQL connection string
- `HIVETALK_URL` - The hivetalk url

## Files

- `main.go` - Main entry point that checks for upcoming events
- `utils.go` - Utility functions for Nostr event handling and database operations

## Building and Running

```bash
# Build the binary
go build -o send30311

# Run the send events binary
./send30311
```

The send events binary will:
1. Check for events starting or ending within a 2-minute window
2. Send appropriate Nostr events for starting/ending meetings
3. Update the event status in the database
