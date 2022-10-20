#!/bin/env ash

mkdir -p /var/lib/grafana/plugins
cp -r /umh-datasource /var/lib/grafana/plugins/umh-datasource
cp -r /umh-factoryinput-panel /var/lib/grafana/plugins/umh-factoryinput-panel
cp -r /umh-datasource-v2 /var/lib/grafana/plugins/umh-datasource-v2

echo "Grafana plugins extracted"
ls -la /var/lib/grafana/plugins
echo "UMH-datasource:"
ls -la /var/lib/grafana/plugins/umh-datasource
echo "UMH-factoryinput-panel:"
ls -la /var/lib/grafana/plugins/umh-factoryinput-panel
echo "UMH-datasource-v2:"
ls -la /var/lib/grafana/plugins/umh-datasource-v2