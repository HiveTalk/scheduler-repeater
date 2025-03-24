# HiveTalk Send Notes Service

This service monitors a PostgreSQL database table for scheduled Nostr notes and sends them to specified relays at the scheduled time.

## Features

- Polls database for pending notes that are due to be sent
- Processes notes in batches with concurrent workers
- Updates note status after sending (published/failed)
- Records errors and publishing timestamps
- Logs activities to a text file

## Setup

1. Make sure you have Go 1.20 or later installed
2. Copy `.env.example` to `.env` and configure your database connection string
3. Build the service: `go build -o send_notes`
4. Test the service: `./send_notes`

## Deployment

To deploy as a systemd service:

1. Copy the binary to your server (e.g., `/opt/hivetalk/scheduler/send_notes/`)
2. Copy the `.env` file to the same directory
3. Copy `send_notes.service` to `/etc/systemd/system/`
4. Enable and start the service:
   ```
   sudo systemctl daemon-reload
   sudo systemctl enable send_notes.service
   sudo systemctl start send_notes.service
   ```

## Logs

Logs are stored in the `logs` directory with the naming format `send_notes_YYYY-MM-DD.log`.

## Database Schema

The service works with the `scheduled_notes` table which has the following schema:

```sql
CREATE TABLE public.scheduled_notes (
    id uuid NOT NULL DEFAULT extensions.uuid_generate_v4(),
    created_at timestamp with time zone NOT NULL DEFAULT timezone('utc'::text, now()),
    updated_at timestamp with time zone NOT NULL DEFAULT timezone('utc'::text, now()),
    profile_id uuid NOT NULL,
    content text NOT NULL,
    scheduled_for timestamp with time zone NOT NULL,
    published_at timestamp with time zone NULL,
    status text NOT NULL DEFAULT 'pending'::text,
    relay_urls text[] NOT NULL DEFAULT '{}'::text[],
    event_id text NULL,
    error_message text NULL,
    signature text NULL,
    signed_event text NULL,
    CONSTRAINT scheduled_notes_pkey PRIMARY KEY (id),
    CONSTRAINT scheduled_notes_profile_id_fkey FOREIGN KEY (profile_id) REFERENCES profiles(id) ON DELETE CASCADE,
    CONSTRAINT valid_status CHECK ((status = ANY (ARRAY['pending'::text, 'published'::text, 'failed'::text])))
);
```
