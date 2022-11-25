#!/bin/env ash

sleep 30

# Check if /opt/hivemq/extensions/hivemq-allow-all-extensions exists

if [ -d "/opt/hivemq-ce-2022.1/extensions/hivemq-file-rbac-extension" ]; then
    echo "Removing /opt/hivemq-ce-2022.1/extensions/hivemq-file-rbac-extension"
    rm -rf /opt/hivemq-ce-2022.1/extensions/hivemq-file-rbac-extension
    kill -TERM 1
fi