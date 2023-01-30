#!/bin/env ash

# Copy hivemq-file-rbac-extension only if RBAC_ENABLED is true
if [ "$RBAC_ENABLED" = "true" ]; then
    echo "RBAC is enabled"
    cp -r /hivemq-file-rbac-extension /opt/hivemq-ce-2022.1/extensions/hivemq-file-rbac-extension
else
    echo "RBAC is disabled. Skipping hivemq-file-rbac-extension"
    cp -r /hivemq-allow-all-extension /opt/hivemq-ce-2022.1/extensions/hivemq-allow-all-extension
fi
cp -r /hivemq-prometheus-extension /opt/hivemq-ce-2022.1/extensions/hivemq-prometheus-extension
cp -r /hivemq-heartbeat-extension /opt/hivemq-ce-2022.1/extensions/hivemq-heartbeat-extension


echo "Listing extensions directory"
ls -la /opt/hivemq-ce-2022.1/extensions