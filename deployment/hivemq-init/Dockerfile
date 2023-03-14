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

FROM hivemq/hivemq-ce:2022.1 as hivemq
FROM alpine as base

# Download hivemq-file-rbac-extension
ADD "https://github.com/hivemq/hivemq-file-rbac-extension/releases/download/4.5.3/hivemq-file-rbac-extension-4.5.3.zip" hivemq-file-rbac-extension.zip
RUN echo "ce5b037c0f15787360a9a674c248c1e1e35f2375e585c7cc45af609c47030b5e  hivemq-file-rbac-extension.zip" | sha256sum -c

ADD "https://github.com/hivemq/hivemq-prometheus-extension/releases/download/4.0.7/hivemq-prometheus-extension-4.0.7.zip" hivemq-prometheus-extension.zip
RUN echo "dc06cf7753bbc35e4fee82413275fb72066f2c92e483857f05ea79d7fb1e403a  hivemq-prometheus-extension.zip" | sha256sum -c

ADD "https://github.com/hivemq/hivemq-heartbeat-extension/releases/download/1.0.4/hivemq-heartbeat-extension-1.0.4.zip" hivemq-heartbeat-extension.zip
RUN echo "1d8da02583aae93f37042a4564e9c30ff5551cc08042e5031b34d309fda3538e  hivemq-heartbeat-extension.zip" | sha256sum -c

RUN unzip -n hivemq-file-rbac-extension.zip
RUN unzip -n hivemq-prometheus-extension.zip
RUN unzip -n hivemq-heartbeat-extension.zip

COPY ./deployment/hivemq-init/runner.sh /runner.sh
RUN dos2unix /runner.sh

FROM alpine as runner
COPY --from=hivemq /opt/hivemq/extensions/hivemq-allow-all-extension /hivemq-allow-all-extension
COPY --from=base /hivemq-file-rbac-extension /hivemq-file-rbac-extension
COPY --from=base /hivemq-prometheus-extension /hivemq-prometheus-extension
COPY --from=base /hivemq-heartbeat-extension /hivemq-heartbeat-extension
COPY --from=base /runner.sh /runner.sh

RUN chmod +x /runner.sh

ENTRYPOINT ["ash","/runner.sh"]
