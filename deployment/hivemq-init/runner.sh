#!/bin/env ash

mkdir -p /opt/hivemq-ce-2022.1/extensions

cp -r /hivemq-file-rbac-extension /opt/hivemq-ce-2022.1/extensions/hivemq-file-rbac-extension
cp /cleaner.sh /opt/hivemq-ce-2022.1/extensions/cleaner.sh