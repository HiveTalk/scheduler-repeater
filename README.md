# Hivetalk backend Repeater

There are multiple scrips in this repository

- vanilla_30312: This script posts 30312 room open and closes for the hivetalksfu server.
It will only post for rooms that are opened with a pubkey moderator. 

- honey_30312: while honey automatically publishes to at least two relays, this script also possts 30312 events to bigger relays, for all rooms open and closes. 

- Discord: This script posts 30311, 30312, 30313 events sent to select nostr relays to discord dev channels. 
- send_notes: This service monitors a PostgreSQL database table for scheduled Nostr notes and sends them to specified relays at the scheduled time.


## Future plans

If detailed logging needed, we structure a table for addiitonal status.

On success, db entry will be marked as such and 
not be sent again if the row is selected in next poll. 

If retry is required, on next poll we will attempt. 

If send is failed, it will be marked as failed. 

# service file
in /etc/systemd/system/scheduler.service

# timer file

in /etc/systemd/system/scheduler.timer


# example services running

systemctl status scheduler
systemctl status hivetalk-discord.service

