# Scheduler

The scheduler purpose is to send nostr events at start_time and end_time. In this case we use systemd timer with systemd service to handle future tasks. This is just demo code, rewrite it in golang as js sucks

When the start time arrives, within 2 minutes of the actual time, we select the event from the database and send it to the relay. After sending, we mark the status of the send in the database.
