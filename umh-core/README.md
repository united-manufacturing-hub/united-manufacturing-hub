# README

See also [docs](docs/README.md)

## FSMv2 Communicator

The FSMv2 communicator is a new implementation of the backend communication layer using finite state machine architecture. It provides improved state management and error handling compared to the legacy Puller/Pusher implementation.

### Enabling FSMv2 Communicator

#### Option 1: Environment Variables

```bash
docker run -d \
  -e AUTH_TOKEN=your-auth-token \
  -e API_URL=https://management.umh.app \
  -e USE_FSMV2_TRANSPORT=true \
  umh-core:latest
```

| Variable | Required | Description |
|----------|----------|-------------|
| `AUTH_TOKEN` | Yes | Authentication token from Management Console |
| `API_URL` | Yes | Backend relay server URL (e.g., `https://management.umh.app`) |
| `USE_FSMV2_TRANSPORT` | Yes | Set to `true` to enable FSMv2 communicator |

#### Option 2: config.yaml

```yaml
agent:
  communicator:
    apiUrl: "https://management.umh.app"
    authToken: "your-auth-token"
    useFSMv2Transport: true
```

### Disabling FSMv2 Communicator

To revert to the legacy communicator, explicitly set:

```bash
-e USE_FSMV2_TRANSPORT=false
```

Or in config.yaml:

```yaml
agent:
  communicator:
    useFSMv2Transport: false
```

**Note:** If `useFSMv2Transport` was previously enabled and saved to config.yaml, you must explicitly set it to `false` - simply omitting the environment variable will not override the persisted config value.

### Verifying FSMv2 Communicator

To verify that FSMv2 communicator is enabled and working:

#### Check Metrics

```bash
curl localhost:8080/metrics | grep fsmv2
```

You should see metrics like `umh_fsmv2_*` indicating the FSMv2 supervisor is running.

#### Watch Communicator Logs

```bash
cat /data/logs/umh-core/current | grep fsmv2
```

Or to follow logs in real-time:

```bash
tail -f /data/logs/umh-core/current | grep fsmv2
```

You should see log entries with `[fsmv2]` prefix showing state transitions and sync operations.
