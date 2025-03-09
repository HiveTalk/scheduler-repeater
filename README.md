# Hivetalk backend-scheduler

There are multiple scrips in this repository

- vanilla_30312: This script posts 30312 room open and closes for the hivetalksfu server.
It will only post for rooms that are opened with a pubkey moderator. 

- Discord: This script posts 30311, 30312, 30313 events sent to select nostr relays to discord dev channels. 

- 30311_events
This scheduler is for for posting nostr events to relays from the server side, 
using the db as the datasource for signing, formatting and sending.


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

