FROM alpine as base

# Download hivemq-file-rbac-extension
ADD "https://github.com/hivemq/hivemq-file-rbac-extension/releases/download/4.5.3/hivemq-file-rbac-extension-4.5.3.zip" hivemq-file-rbac-extension.zip
RUN echo "ce5b037c0f15787360a9a674c248c1e1e35f2375e585c7cc45af609c47030b5e  hivemq-file-rbac-extension.zip" | sha256sum -c

RUN unzip -n hivemq-file-rbac-extension.zip

COPY deployment/grafana-plugin-extractor/runner.sh /runner.sh
RUN dos2unix /runner.sh

FROM alpine as runner
COPY --from=base /hivemq-file-rbac-extension /hivemq-file-rbac-extension
COPY --from=base /runner.sh /runner.sh

RUN chmod +x /runner.sh

ENTRYPOINT ["ash","/runner.sh"]