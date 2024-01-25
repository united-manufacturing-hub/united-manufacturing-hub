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

ARG GRAFANA_VERSION="9.3.1"

FROM docker.io/grafana/grafana:${GRAFANA_VERSION} as builder

ARG GF_INSTALL_IMAGE_RENDERER_PLUGIN="false"

ARG GF_GID="0"

ENV GF_PATHS_PLUGINS="/etc/grafana/plugins"
ENV GF_PLUGIN_RENDERING_CHROME_BIN="/usr/bin/chrome"

USER root

# Echoing for debugging purposes
RUN echo "Creating plugins directory at ${GF_PATHS_PLUGINS}" && \
    mkdir -p "$GF_PATHS_PLUGINS" && \
    echo "Changing ownership of ${GF_PATHS_PLUGINS} to grafana:${GF_GID}" && \
    chown -R grafana:${GF_GID} "$GF_PATHS_PLUGINS" && \
    if [ $GF_INSTALL_IMAGE_RENDERER_PLUGIN = "true" ]; then \
      echo "Installing image renderer plugin" && \
      if grep -i -q alpine /etc/issue; then \
        apk add --no-cache udev ttf-opensans chromium && \
        ln -s /usr/bin/chromium-browser "$GF_PLUGIN_RENDERING_CHROME_BIN"; \
        echo "Installed chromium for Alpine"; \
      else \
        cd /tmp && \
        echo "Downloading Google Chrome for Debian-based distributions" && \
        curl -sLO https://dl.google.com/linux/direct/google-chrome-stable_current_amd64.deb && \
        DEBIAN_FRONTEND=noninteractive && \
        apt-get update -q && \
        apt-get install -q -y --no-install-recommends ./google-chrome-stable_current_amd64.deb && \
        rm -rf /var/lib/apt/lists/* && \
        rm ./google-chrome-stable_current_amd64.deb && \
        ln -s /usr/bin/google-chrome "$GF_PLUGIN_RENDERING_CHROME_BIN"; \
        echo "Installed Google Chrome"; \
      fi \
    fi

USER grafana

RUN if [ $GF_INSTALL_IMAGE_RENDERER_PLUGIN = "true" ]; then \
      echo "Installing grafana-image-renderer plugin" && \
      grafana-cli \
        --pluginsDir "$GF_PATHS_PLUGINS" \
        --pluginUrl https://github.com/grafana/grafana-image-renderer/releases/latest/download/plugin-linux-x64-glibc-no-chromium.zip \
        plugins install grafana-image-renderer; \
    fi

ARG GF_INSTALL_PLUGINS="https://github.com/united-manufacturing-hub/united-manufacturing-hub-datasource/releases/download/v1.2.1/umh-datasource.zip;umh-datasource,https://github.com/united-manufacturing-hub/umh-factoryinput-panel/releases/download/v1.0.3/umh-factoryinput-panel.zip;umh-factoryinput-panel,https://github.com/united-manufacturing-hub/umh-datasource-v2/releases/download/v1.1.4/umh-v2-datasource.zip;umh-v2-datasource,grafana-worldmap-panel,grafana-piechart-panel,aceiot-svg-panel,grafana-worldmap-panel,natel-discrete-panel,isaozler-paretochart-panel,williamvenner-timepickerbuttons-panel,agenty-flowcharting-panel,marcusolsson-dynamictext-panel,factry-untimely-panel,cloudspout-button-panel"

RUN if [ ! -z "${GF_INSTALL_PLUGINS}" ]; then \
      OLDIFS=$IFS; \
      IFS=','; \
      echo "Starting plugin installation" && \
      for plugin in ${GF_INSTALL_PLUGINS}; do \
        IFS=$OLDIFS; \
        if expr match "$plugin" '.*\;.*'; then \
          pluginUrl=$(echo "$plugin" | cut -d';' -f 1); \
          pluginInstallFolder=$(echo "$plugin" | cut -d';' -f 2); \
          echo "Installing plugin from URL ${pluginUrl} to folder ${pluginInstallFolder}" && \
          grafana-cli --pluginUrl ${pluginUrl} --pluginsDir "${GF_PATHS_PLUGINS}" plugins install "${pluginInstallFolder}"; \
          # Echoing for debugging: check if the plugin installation was successful
          if [ $? -ne 0 ]; then \
              echo "Failed to install plugin from URL ${pluginUrl}"; \
              exit 1; \
          fi \
        else \
          echo "Installing plugin ${plugin}" && \
          grafana-cli --pluginsDir "${GF_PATHS_PLUGINS}" plugins install ${plugin}; \
        fi \
      done \
      echo "Plugin installation completed"; \
    fi

FROM alpine as final

COPY --from=builder /etc/grafana/plugins /plugins

