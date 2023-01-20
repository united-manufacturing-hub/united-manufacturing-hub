ARG GRAFANA_VERSION="8.5.9"

FROM grafana/grafana:${GRAFANA_VERSION} as builder

ARG GF_INSTALL_IMAGE_RENDERER_PLUGIN="false"

ARG GF_GID="0"

ENV GF_PATHS_PLUGINS="/etc/grafana/plugins"
ENV GF_PLUGIN_RENDERING_CHROME_BIN="/usr/bin/chrome"

USER root

RUN mkdir -p "$GF_PATHS_PLUGINS" && \
    chown -R grafana:${GF_GID} "$GF_PATHS_PLUGINS" && \
    if [ $GF_INSTALL_IMAGE_RENDERER_PLUGIN = "true" ]; then \
      if grep -i -q alpine /etc/issue; then \
        apk add --no-cache udev ttf-opensans chromium && \
        ln -s /usr/bin/chromium-browser "$GF_PLUGIN_RENDERING_CHROME_BIN"; \
      else \
        cd /tmp && \
        curl -sLO https://dl.google.com/linux/direct/google-chrome-stable_current_amd64.deb && \
        DEBIAN_FRONTEND=noninteractive && \
        apt-get update -q && \
        apt-get install -q -y ./google-chrome-stable_current_amd64.deb && \
        rm -rf /var/lib/apt/lists/* && \
        rm ./google-chrome-stable_current_amd64.deb && \
        ln -s /usr/bin/google-chrome "$GF_PLUGIN_RENDERING_CHROME_BIN"; \
      fi \
    fi

USER grafana

RUN if [ $GF_INSTALL_IMAGE_RENDERER_PLUGIN = "true" ]; then \
      grafana-cli \
        --pluginsDir "$GF_PATHS_PLUGINS" \
        --pluginUrl https://github.com/grafana/grafana-image-renderer/releases/latest/download/plugin-linux-x64-glibc-no-chromium.zip \
        plugins install grafana-image-renderer; \
    fi

ARG GF_INSTALL_PLUGINS="https://github.com/united-manufacturing-hub/united-manufacturing-hub-datasource/releases/download/v1.2.1/umh-datasource.zip;umh-datasource,https://github.com/united-manufacturing-hub/umh-factoryinput-panel/releases/download/v1.0.3/umh-factoryinput-panel.zip;umh-factoryinput-panel,https://github.com/united-manufacturing-hub/umh-datasource-v2/releases/download/v1.0.92/umh-v2-datasource.zip;umh-v2-datasource,grafana-worldmap-panel,grafana-piechart-panel,aceiot-svg-panel,grafana-worldmap-panel,natel-discrete-panel,isaozler-paretochart-panel,williamvenner-timepickerbuttons-panel,agenty-flowcharting-panel,marcusolsson-dynamictext-panel,factry-untimely-panel,cloudspout-button-panel"

RUN if [ ! -z "${GF_INSTALL_PLUGINS}" ]; then \
      OLDIFS=$IFS; \
      IFS=','; \
      for plugin in ${GF_INSTALL_PLUGINS}; do \
        IFS=$OLDIFS; \
        if expr match "$plugin" '.*\;.*'; then \
          pluginUrl=$(echo "$plugin" | cut -d';' -f 1); \
          pluginInstallFolder=$(echo "$plugin" | cut -d';' -f 2); \
          grafana-cli --pluginUrl ${pluginUrl} --pluginsDir "${GF_PATHS_PLUGINS}" plugins install "${pluginInstallFolder}"; \
        else \
          grafana-cli --pluginsDir "${GF_PATHS_PLUGINS}" plugins install ${plugin}; \
        fi \
      done \
    fi

FROM alpine as final

COPY --from=builder /etc/grafana/plugins /plugins