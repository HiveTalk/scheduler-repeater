# HiveTalk Scheduler Systemd Configuration

This directory contains systemd service and timer configurations for the HiveTalk event scheduler.

## Files
- `hivetalk-scheduler.service`: Defines how to run the scheduler
- `hivetalk-scheduler.timer`: Defines when to run the scheduler

## Installation

1. Copy files to systemd directory:
```bash
sudo cp hivetalk-scheduler.* /etc/systemd/system/
```

2. Reload systemd daemon:
```bash
sudo systemctl daemon-reload
```

3. Enable and start the timer:
```bash
sudo systemctl enable hivetalk-scheduler.timer
sudo systemctl start hivetalk-scheduler.timer
```

## Monitoring

Check timer status:
```bash
systemctl status hivetalk-scheduler.timer
```

Check service status:
```bash
systemctl status hivetalk-scheduler.service
```

View logs:
```bash
journalctl -u hivetalk-scheduler.service
```

## Timer Design

The timer is configured to run every minute with these key features:

1. **Execution Window**: 
   - The scheduler checks for events in a ±2 minute window
   - Running every minute ensures overlap between windows
   - No events will be missed as long as they're in the database

2. **Load Management**:
   - RandomizedDelaySec=5 adds a small random delay (0-5s)
   - Prevents exact-minute execution to reduce system load

3. **Reliability**:
   - Persistent=true ensures missed executions run on system startup
   - AccuracySec=1s maintains precise timing
   - TimeoutStartSec=180 allows up to 3 minutes for processing

## Timing Diagram

```
Minute 1:  |----[±2min window]----| 
Minute 2:      |----[±2min window]----| 
Minute 3:          |----[±2min window]----| 
```

This overlapping window design ensures:
- Each event has multiple chances to be processed
- System failures under 3 minutes won't cause missed events
- Load is distributed across time

## Process detail

Description of the process in more detail, plus rationale.

### Window Overlap

Each run checks ±2 minutes (4-minute window)
Running every minute means each event has 4 chances to be processed

Example: An event at 12:00:00
Caught by 11:58 run (11:58-12:02 window)
Caught by 11:59 run (11:59-12:03 window)
Caught by 12:00 run (12:00-12:04 window)
Caught by 12:01 run (12:01-12:05 window)

## Safety Features

- RandomizedDelaySec prevents thundering herd
- Persistent ensures no missed events after downtime
- 3-minute timeout accommodates processing up to 50 events

## Load Distribution

- Random 5-second delay spreads load
- Each minute processes only new events in window
- Overlapping windows provide redundancy
