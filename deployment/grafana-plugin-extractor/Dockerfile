FROM alpine as base

# Download umh-datasource
ADD "https://github.com/united-manufacturing-hub/united-manufacturing-hub-datasource/releases/latest/download/umh-datasource.zip" umh-datasource.zip
ADD "https://github.com/united-manufacturing-hub/united-manufacturing-hub-datasource/releases/latest/download/umh-datasource.zip.md5" umh-datasource.zip.md5
# Check md5 checksum
RUN md5sum -c umh-datasource.zip.md5

# Download umh-factoryinput-panel
ADD "https://github.com/united-manufacturing-hub/umh-factoryinput-panel/releases/latest/download/umh-factoryinput-panel.zip" umh-factoryinput-panel.zip
ADD "https://github.com/united-manufacturing-hub/umh-factoryinput-panel/releases/latest/download/umh-factoryinput-panel.zip.md5" umh-factoryinput-panel.zip.md5
# Check md5 checksum
RUN md5sum -c umh-factoryinput-panel.zip.md5

# Download umh-datasource-v2
ADD "https://github.com/united-manufacturing-hub/umh-datasource-v2/releases/latest/download/umh-datasource-v2.zip" umh-datasource-v2.zip
ADD "https://github.com/united-manufacturing-hub/umh-datasource-v2/releases/latest/download/umh-datasource-v2.zip.md5" umh-datasource-v2.zip.md5
# Check md5 checksum
RUN md5sum -c umh-datasource.zip.md5

# Unpack grafana plugins
RUN unzip -n umh-datasource.zip
RUN unzip -n umh-factoryinput-panel.zip
RUN unzip -n umh-datasource-v2.zip
RUN mv umh-datasource-v2 umh-v2-datasource


COPY deployment/grafana-plugin-extractor/runner.sh /runner.sh
RUN dos2unix /runner.sh

FROM alpine as runner
COPY --from=base /umh-datasource /umh-datasource
COPY --from=base /umh-factoryinput-panel /umh-factoryinput-panel
COPY --from=base /umh-v2-datasource /umh-v2-datasource
COPY --from=base /runner.sh /runner.sh

RUN chmod +x /runner.sh

ENTRYPOINT ["ash","/runner.sh"]