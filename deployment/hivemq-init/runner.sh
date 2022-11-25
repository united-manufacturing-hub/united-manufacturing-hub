#!/bin/env ash

# Delete hivemq-allow-all-extensions
rm -rf /opt/hivemq-ce-2022.1/extensions/hivemq-allow-all-extensions


cp -r /hivemq-file-rbac-extension /opt/hivemq-ce-2022.1/extensions/hivemq-file-rbac-extension
