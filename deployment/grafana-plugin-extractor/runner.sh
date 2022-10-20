#!/bin/env ash

mkdir -p /var/lib/grafana/plugins

# Delete old versions if exist
rm -rf /var/lib/grafana/plugins/umh-datasource
rf -rf /var/lib/grafana/plugins/umh-factoryinput-panel
rf -rf /var/lib/grafana/plugins/umh-v2-datasource

cp -r /umh-datasource /var/lib/grafana/plugins/umh-datasource
cp -r /umh-factoryinput-panel /var/lib/grafana/plugins/umh-factoryinput-panel
cp -r /umh-v2-datasource /var/lib/grafana/plugins/umh-v2-datasource

echo "Grafana plugins extracted"
ls -la /var/lib/grafana/plugins

echo "UMH-datasource:"
ls -la /var/lib/grafana/plugins/umh-datasource

echo "UMH-factoryinput-panel:"
ls -la /var/lib/grafana/plugins/umh-factoryinput-panel

echo "UMH-v2-datasource:"
ls -la /var/lib/grafana/plugins/umh-v2-datasource