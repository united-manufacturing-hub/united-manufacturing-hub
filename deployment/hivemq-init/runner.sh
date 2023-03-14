#!/bin/env ash
# Copyright 2023 UMH Systems GmbH
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.


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