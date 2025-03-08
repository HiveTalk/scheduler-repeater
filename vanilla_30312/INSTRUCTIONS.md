# HiveTalk Poller Instructions

This Golang script polls the HiveTalk Vanilla API to monitor rooms and broadcasts their status to Nostr relays as 30312 events.

## Setup

1. Make sure you have Go installed on your system (version 1.16 or later recommended).

2. Install the required dependencies:

```bash
go mod init hivetalk_poller
go get github.com/joho/godotenv
go get github.com/nbd-wtf/go-nostr
```

3. Configure your `.env` file with the following variables (you can copy from `env.example`):

```sh
RELAY_URLS='wss://honey.nostr1.com','wss://hivetalk.nostr1.com'
NOSTR_PVT_KEY='your-private-key-for-nostr-bot'
HIVETALK_API_KEY='your-hivetalk-api-key'
BASE_URL='https://hivetalk.org'
```

## Running the Script

To run the script:

```bash
go run hivetalk_poller.go
```

The script will:
- Poll the HiveTalk API every minute
- Track rooms with Nostr pubkey moderators
- Broadcast 30312 events when rooms open or close
- Store room information in a local `rooms.json` file

## Running as a Service

To run the script as a background service, you can use various methods depending on your operating system:

### Using systemd (Linux)

Create a service file at `/etc/systemd/system/hivetalk-poller.service`:

```
[Unit]
Description=HiveTalk Poller Service
After=network.target

[Service]
Type=simple
User=yourusername
WorkingDirectory=/path/to/vanilla_30312
ExecStart=/bin/bash /path/to/vanilla_30312/run.sh
Restart=on-failure
RestartSec=10
StandardOutput=syslog
StandardError=syslog
SyslogIdentifier=hivetalk-poller

[Install]
WantedBy=multi-user.target
```

Then enable and start the service:

```bash
sudo systemctl enable hivetalk-poller
sudo systemctl start hivetalk-poller
```

### Using launchd (macOS)

Create a plist file at `~/Library/LaunchAgents/com.hivetalk.poller.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.hivetalk.poller</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/go/bin/go</string>
        <string>run</string>
        <string>/path/to/vanilla_30312/hivetalk_poller.go</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>WorkingDirectory</key>
    <string>/path/to/vanilla_30312</string>
    <key>StandardErrorPath</key>
    <string>/path/to/vanilla_30312/error.log</string>
    <key>StandardOutPath</key>
    <string>/path/to/vanilla_30312/output.log</string>
</dict>
</plist>
```

Then load the service:

```bash
launchctl load ~/Library/LaunchAgents/com.hivetalk.poller.plist
```

## Building an Executable

To build a standalone executable:

```bash
go build -o hivetalk_poller hivetalk_poller.go
```

Then you can run it directly:

```bash
./hivetalk_poller
```
