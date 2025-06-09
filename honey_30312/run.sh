#!/bin/bash
cd /root/scheduler/honey_30312

# Make sure Go is in the PATH
export PATH=$PATH:/usr/local/go/bin

# Check if the Go binary exists, if not, compile it
if [ ! -f "./honey_poller" ] || [ "$(file ./honey_poller | grep -i 'executable')" == "" ]; then
    echo "Compiling honey_poller binary..."
    go build -o honey_poller
fi

# Run the binary
./honey_poller