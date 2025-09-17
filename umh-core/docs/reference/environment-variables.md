# Environment Variables

This is the reference for all environment variables used by umh-core. These variables override or supplement configuration from `config.yaml`.

| Name             | Values                                                                 | Description                                          |
| ---------------- | ---------------------------------------------------------------------- | ---------------------------------------------------- |
| LOGGING\_LEVEL   | PRODUCTION, DEBUG                                                      | Controls log verbosity. DEBUG shows detailed internal operations |
| AUTH\_TOKEN      | (Base64 encoded token)                                                 | Management Console authentication token. Overrides auth token from config.yaml |
| API\_URL         | https://management.umh.app/api, https://staging.management.umh.app/api | Management Console API endpoint. Use staging for testing environments |
| RELEASE\_CHANNEL | enterprise, stable, nightly                                            | Auto-update channel. Enterprise = most stable, nightly = latest features |
| ALLOW\_INSECURE\_TLS | true, false                                                        | Skip TLS certificate verification. Use for corporate firewalls with MITM proxies |
| LOCATION\_<0-6>  | (string values)                                                        | Sets Agent location hierarchy levels 0-6. Example: LOCATION_0=factory, LOCATION_1=line1 |
| S6\_PERSIST\_DIRECTORY | true/1/TRUE/True, (unset/false/other)                           | Controls S6 service directory persistence. Truthy values (true, 1, TRUE, etc.) = /data/services (persist across container restarts for debugging). Default/false = /tmp/umh-core-services (cleared on container restart for fresh state) |

