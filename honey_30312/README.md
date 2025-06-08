# README

This script will poll the BASE_URL endpoint every 60 seconds, which will return data in this format: 

```json

[{"name":"Hive Room","sid":"RM_v4HsDDo2H4Rd","createdAt":"2025-06-08T02:51:09Z","numParticipants":1,"description":"People who work on Hivetalk ","pictureUrl":"https://honey.hivetalk.org/_image?href=%2F_astro%2Fhivetalkbg2.CXhLVsIP.png","status":"open"}]
```

The script will then publish a 30312 event for each room, with the following tags:

- d tag
- room tag
- summary tag
- status tag
- image tag
- service tag
- t tag
- t tag
- relays tag

to the relays listed in the `RELAY_URLS` environment variable, using the private key listed in the `NOSTR_PVT_KEY` environment variable.

If there is a DISCORD_URL environment variable, the script will also send a message to this discord webhook in the same way as the discord/nostr_listener.go code does, but include a delay if there are too many messages to send to webhook in order to prevent throttling by discord.