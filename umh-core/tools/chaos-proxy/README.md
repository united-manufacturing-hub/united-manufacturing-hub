# Chaos Proxy

A reverse proxy that injects network faults between umh-core and the Management Console backend. Used to test the resilience of the transport workers (push/pull) under degraded network conditions.

## Why this exists

Real-world edge deployments experience unreliable networks: packet loss, high latency, mid-stream TCP resets, NAT table expirations, and overloaded upstream proxies. This tool reproduces those conditions in a controlled way to verify that the communicator's transport workers handle failures gracefully.

## Prerequisites

- Docker and docker compose (v2)
- An auth token for an umh-core instance
- The umh-core container image (or build one locally)

## Quick start

```bash
# Set required environment variables
export AUTH_TOKEN="your-auth-token"
export UMH_CORE_IMAGE="ghcr.io/united-manufacturing-hub/united-manufacturing-hub:latest"

# Run a predefined scenario
./scenarios/scenario1-drops.sh

# Or run with custom flags
CHAOS_PROXY_FLAGS="--drop-every=5 --long-poll --long-poll-mu=7.0" docker compose up
```

## Proxy CLI flags

| Flag | Default | Description |
|------|---------|-------------|
| `--listen` | `:8090` | Address the proxy listens on |
| `--target` | `https://management.umh.app` | Upstream URL to forward requests to |
| `--drop-every` | `3` | Drop every N-th connection by sending EOF (end of file / TCP FIN) (0 = disable) |
| `--long-poll` | `false` | Enable long-poll delay simulation before proxying |
| `--long-poll-mu` | `8.5` | Lognormal mu parameter (ln of median delay in ms; 8.5 ~ 4.9s median) |
| `--long-poll-sigma` | `1.2` | Lognormal sigma parameter (spread; higher = more variance) |
| `--long-poll-cap` | `31000` | Maximum delay in ms (caps lognormal outliers) |
| `--long-poll-kill-pct` | `20` | Percentage chance (0-100) to kill connection mid-delay |
| `--long-poll-method` | `` (all) | Only apply long-poll to this HTTP method (e.g., `GET`) |
| `--long-poll-path` | `` (all) | Only apply long-poll to requests whose path contains this substring (e.g., `/v2/instance/pull`) |

## Scenarios

| Scenario | Script | Key Flags | What It Tests | Expected Behavior | Pass Criteria |
|----------|--------|-----------|---------------|-------------------|---------------|
| 1. Drops | `scenario1-drops.sh` | `--drop-every=3` | EOF on every 3rd connection | Workers detect EOF and retry with backoff | Instance stays online, retries visible in metrics |
| 2. Long-Poll | `scenario2-longpoll.sh` | `--long-poll --long-poll-mu=8.5 --long-poll-sigma=1.2 --long-poll-cap=31000 --long-poll-kill-pct=20` | Slow responses + 20% mid-stream kills | Context deadlines fire, killed requests retried | No goroutine leaks, elevated latency in metrics |
| 3. Mid-Stream Kills | `scenario3-kills.sh` | `--long-poll --long-poll-mu=8.5 --long-poll-sigma=1.2 --long-poll-kill-pct=30` | 30% of connections killed mid-stream | Broken pipe detected, partial responses discarded | No panics, error metrics recover after kills |
| 4. Combined | `scenario4-combined.sh` | `--drop-every=3 --long-poll --long-poll-mu=8.5 --long-poll-sigma=1.2 --long-poll-cap=31000 --long-poll-kill-pct=20` | All chaos types simultaneously | Mixed failures handled, no unrecoverable state | Instance recovers, no deadlocks, bounded errors |
| 5a. Pull-Only (path) | `scenario5-pull-only.sh` | `--long-poll --long-poll-mu=10.0 --long-poll-sigma=0.3 --long-poll-cap=31000 --long-poll-kill-pct=20 --long-poll-path=/v2/instance/pull` | Pull path delays only, push unaffected | Push healthy, pull degraded | Push metrics normal, instance stays online |
| 5b. Push-Only (path) | `scenario5b-push-only.sh` | `--long-poll --long-poll-mu=10.0 --long-poll-sigma=0.3 --long-poll-cap=31000 --long-poll-kill-pct=20 --long-poll-path=/v2/instance/push` | Push path delays only, pull unaffected | Pull healthy, push degraded | Pull metrics normal, actions received normally |

## Collecting data

### Prometheus metrics (port 8080)

```bash
# Scrape all metrics (port 8080 is exposed in docker-compose.yml)
curl -s http://localhost:8080/metrics

# Watch transport worker metrics
watch -n5 'curl -s http://localhost:8080/metrics | grep -E "transport_worker|communicator"'
```

### Debug state (port 8090)

Note: When running the chaos proxy, port 8090 is used by the proxy itself. To access the umh-core debug endpoint, map it to a different host port in docker-compose.yml.

### S6 logs

```bash
# Attach to the umh-core container and read logs
docker compose exec umh-core sh -c 'cat /data/logs/umh-core/current'

# Follow logs in real-time
docker compose exec umh-core sh -c 'tail -f /data/logs/umh-core/current'
```

## Pass/fail criteria checklist

- [ ] Instance appears "online" in Management Console (or recovers within 2 minutes after chaos stops)
- [ ] No panics or goroutine leaks in umh-core logs
- [ ] Error counters in Prometheus metrics are bounded (not growing unbounded)
- [ ] Metrics endpoint (:8080) remains responsive throughout the test
- [ ] Push and pull workers retry with backoff (visible in logs)
- [ ] No data corruption (partial responses not processed as valid)
- [ ] Heartbeat continues even under heavy chaos

## Known limitations

- **TLS topology differs from production**: In production, umh-core connects directly to
  management.umh.app over HTTPS. In the chaos test setup, umh-core connects to the proxy
  over HTTP, and the proxy connects to management.umh.app over HTTPS. TLS-specific failure
  modes (certificate errors, handshake timeouts) are not exercised.
- **No bandwidth throttling**: The proxy can drop, delay, or kill connections, but cannot
  simulate slow throughput (e.g., 56kbps links).
- **No response corruption**: The proxy either forwards the full response or kills the
  connection. Partial/truncated responses are not simulated.

## Chaos test results

Recorded test results and analysis are stored in:
`000_human_artifacts/fsmv2/transport/chaos/`
