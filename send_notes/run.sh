#!/bin/bash
cd /root/scheduler/send_notes

# Make sure Go is in the PATH
export PATH=$PATH:/usr/local/go/bin

# Check if the Go binary exists, if not, compile it
if [ ! -f "./send_notes" ] || [ "$(file ./send_notes | grep -i 'executable')" == "" ]; then
    echo "Compiling send_notes binary..."
    go build -o send_notes
fi

# Run the binary
./send_notes