# Honey 30312 HiveTalk Poller

This script polls the HiveTalk API endpoint every 60 seconds to track room status changes (open/closed) and publishes updates to both Nostr relays and Discord (optional).

## How It Works

The poller fetches room data from the BASE_URL endpoint, which returns data in this format:

```json
[
   {"name":"Hive Room",
   "sid":"RM_Dtf94cmbiJPu",
   "createdAt":"2025-06-09T04:32:04Z",
   "numParticipants":1,
   "description":"People who work on Hivetalk ",
   "pictureUrl":"https://honey.hivetalk.org/_image?href=%2F_astro%2Fhivetalkbg2.CXhLVsIP.png","status":"open"},

   {"name":"Witty-Hawk-43",
   "sid":"RM_bEuLoJEtkEER",
   "createdAt":"2025-06-09T04:32:07Z",
   "numParticipants":1
   }
]
```

When room status changes are detected, the script can:

1. **Publish Nostr Events**: Creates and publishes NIP-30312 events with the following tags:
   - d tag (unique identifier)
   - room tag (room name)
   - summary tag (room description or name)
   - status tag (open/closed)
   - image tag (room image if available)
   - service tag (join URL)
   - t tags (for categorization)
   - relays tag

2. **Send Discord Notifications**: Posts room status updates to a Discord webhook with detailed room information.

## Configuration

The script uses environment variables for configuration. Create a `.env` file with the following variables:

```
BASE_URL=https://relay.hivetalk.org/api/list-rooms
NOSTR_PVT_KEY=your_private_key_here
RELAY_URLS=wss://relay1.com,wss://relay2.com
DISCORD_URL=your_discord_webhook_url_here
```

### Optional Integrations

#### Disabling Nostr Integration

Nostr integration can be disabled by either:

1. Setting `NOSTR_PVT_KEY` to empty: `NOSTR_PVT_KEY=`
2. Setting `RELAY_URLS` to empty: `RELAY_URLS=`

When Nostr integration is disabled, the poller will still track room status changes but won't publish any events to relays.

#### Disabling Discord Integration

Discord integration is optional and can be disabled by:

- Omitting the `DISCORD_URL` variable from your `.env` file
- Setting `DISCORD_URL` to empty: `DISCORD_URL=`

## Running the Poller

To run the script directly:

```bash
go run honey_poller.go
```

or build it:

```bash
go build -o honey_poller
```

Or use the compiled binary:

```bash
./honey_poller
```
