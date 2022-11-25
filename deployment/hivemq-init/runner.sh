#!/bin/env ash

echo "Copying hivemq-file-rbac-extension to extensions directory"
cp -r /hivemq-file-rbac-extension /opt/hivemq-ce-2022.1/extensions/hivemq-file-rbac-extension

echo "Copying cleaner script to extensions directory"
cp /cleaner.sh /opt/hivemq-ce-2022.1/extensions/cleaner.sh

echo "Listing extensions directory"
ls -la /opt/hivemq-ce-2022.1/extensions