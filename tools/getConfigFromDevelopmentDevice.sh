#!/bin/bash

if [ -z "$1" ]; then
    echo "No argument supplied"
    exit 1
fi

echo "Running $1"
echo "Resetting SSH key"
ssh-keygen -f "~/.ssh/known_hosts" -R $1
echo "Resetting SSH key done"

CONNECT_STRING="spawn ssh rancher@$1 \"cat /etc/rancher/k3s/k3s.yaml\"; expect \"*:\"; send \"rancher\r\"; interact"

echo "Connecting via SSH"
expect -c "$CONNECT_STRING" | sed -r 's/(\b[0-9]{1,3}\.){3}[0-9]{1,3}\b'/"$1"/
echo "Successful exit"
