# backend-scheduler

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

