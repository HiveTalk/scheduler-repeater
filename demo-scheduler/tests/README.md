# Automated Test data

General usage instructions.

## Auto generate a profile, a room and N events

example to generate events for a room and a new profile:

```sh
# Create 3 events starting in 5 minutes from now
node tests/create_test_data.js 3 --in 5

# Create default number of events (10) with random times
node tests/create_test_data.js

# Create 5 events with random times
node tests/create_test_data.js 5

```

example to select events from db and send to relay if time is within 2 minutes and update status.

```sh
node tests/bulk_send_events.js
```
