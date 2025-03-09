# Discord bot

This script posts 30311, 30312, 30313 events sent to select nostr relays to discord dev channels. 

## Setup

1. Make sure you have Go installed on your system (version 1.16 or later recommended).

2. Install the required dependencies:

```bash
go mod init hivetalk_discord
go get github.com/joho/godotenv
go get github.com/nbd-wtf/go-nostr
```

3. Configure your `.env` file with the following variables (you can copy from `env.example`):

```sh   
RELAY_URL='wss://yourrelayhere'
DISCORD_WEBHOOK='https://discord.com/....'
```

4. Run the script:

```bash
go nostr_listener.go
```

## Running as a Service

To run the script as a background service, you can use various methods depending on your operating system:

### Using systemd (Linux)

Create a service file at `/etc/systemd/system/hivetalk-discord.service`:

```
[Unit]
Description=HiveTalk Discord Bot Service
After=network.target

[Service]
Type=simple
User=yourusername
Group=yourgroupname
EnvironmentFile=/path/to/discord/.env
WorkingDirectory=/path/to/discord
ExecStart=/usr/bin/bash -c 'PATH=/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin /root/scheduler/discord/run.sh'
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
```

Then enable and start the service:

```bash
sudo systemctl enable hivetalk-discord
sudo systemctl start hivetalk-discord
```
