#!/bin/env ash

echo "Copying hivemq-file-rbac-extension to extensions directory"
cp -r /hivemq-file-rbac-extension /opt/hivemq-ce-2022.1/extensions/hivemq-file-rbac-extension
cp -r /hivemq-file-rbac-extension /opt/hivemq-ce-2022.1/extensions/hivemq-prometheus-extension
cp -r /hivemq-file-rbac-extension /opt/hivemq-ce-2022.1/extensions/hivemq-heartbeat-extension


echo "Listing extensions directory"
ls -la /opt/hivemq-ce-2022.1/extensions