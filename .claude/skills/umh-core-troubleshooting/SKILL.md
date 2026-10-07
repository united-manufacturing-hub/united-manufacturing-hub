---
name: umh-core-troubleshooting
description: Use when investigating a umh-core support issue - an instance shown offline, a bridge stuck in "starting", data not flowing, a Linear support ticket with customer logs, or a bug that may span the Management Console, umh-core and benthos-umh.
---

# Troubleshooting umh-core

For team processes (Linear/Sentry routing, ticket handling), see the `CLAUDE.md` in the internal `troubleshooting` repository. This skill covers what is specific to umh-core.

## Where things are inside the container

| What | Path |
|---|---|
| umh-core log | `/data/logs/umh-core/current` |
| Service logs | `/data/logs/<service>/current` (e.g. `benthos-dataflow-read-protocolconverter-<bridge>`; `ls /data/logs \| grep <bridge>`) |
| Config | `/data/config.yaml` |
| S6 scan directory | `/run/service/` |
| S6 service directories | `/tmp/umh-core-services/` by default; `/data/services/` when `S6_PERSIST_DIRECTORY=true` |
| Generated benthos config | `<service dir>/config/benthos.yaml` |

Log files: `current` is active, `.s` is a clean rotation, `.u` is unfinished (container was killed). Timestamps are TAI64N: pipe through `tai64nlocal`.

## Start every investigation

1. Read the Linear ticket **and all its comments**. The comments carry the real story; the description is often outdated. Transcribe every screenshot (German UI text can hold the key detail).
2. Ask the user for the action logs from the Management Console, what they were trying to do, and timestamps.
3. Check whether it is already fixed: search merged PRs by symptom and error message, not only by component or ticket id. Look at PRs merged between "last worked" and "first failed".
4. Build the timeline while you investigate, not afterwards. One line per event: `[UTC timestamp] - [source] - [event]`, with the exact quote and a `file:line` or PR link. Convert log times to UTC.

```bash
gh pr list --limit 30 --state merged --search "<error message keywords>"
gh pr list --limit 50 --state merged --json mergedAt,title | jq '.[] | select(.mergedAt > "YYYY-MM-DD")'
```

## Check three sources before concluding

1. What the UI shows (may be stale).
2. What the logs say (may be from a different time).
3. What actually happens: throughput, Kafka topics (`rpk topic consume umh.v1.<location>.* --num 10`), metrics.

Bridge "starting" in the UI + logs say running + data in Kafka = a status-display problem, not a functional one.

## Log patterns

| Pattern | Meaning | Source |
|---|---|---|
| `gatekeeper_outbound_channel_full`, `fsmv2_outbound_channel_full` | Status message dropped: outbound channel full. UI shows stale status | `pkg/communicator/pkg/subscriber/subscribers.go` (Sentry warning) |
| `Heartbeat .* send a warning`, `send to many consecutive warnings` | A watched goroutine is degrading | `pkg/communicator/pkg/tools/watchdog/watchdog.go` |
| `Failed to generate status message` | Status generation timed out or failed | `pkg/communicator/pkg/subscriber/subscribers.go` |
| `context deadline exceeded` | Timeout | various FSM components |
| `FSMState=''` | S6 returns nothing: service directory missing or corrupted | S6 |

```bash
tai64nlocal < /data/logs/umh-core/current | grep -E "ERROR|WARN" | tail -200
grep -E "outbound_channel_full|send a warning|Failed to generate status message|context deadline exceeded" /data/logs/umh-core/*
```

Customer log archives are usually 7z: `7z x "*.7z"` (`brew install p7zip`).

## Instance shown offline but running

Usual causes: network instability between the site and the Cloudflare edge, a full outbound channel, MTU/fragmentation, NAT or firewall state timeouts. A restart helps because it clears the queue, resets TCP state and may route through another edge. Sentry: search by error message and by the customer's region; `geo.region` tags are incomplete.

## What a restart tells you

| Restart… | Points to |
|---|---|
| fixes it | state, cache or queue (channel overflow, FSM state) |
| does not fix it | configuration or network (IP, port, routing, credentials) |
| sometimes fixes it | race condition (config sync plus manual edits, several browser tabs, concurrent deploys) |
| fixes it for a while | leak or accumulation (memory, goroutines, growing queue) |

## Tracing across repositories

Path of a deploy: frontend → MC backend (queues the action) → umh-core pull → router → action handler writes `config.yaml` → FSM reconcile → benthos config rendered → S6 starts benthos-umh → device. Status returns the other way. Details: `.claude/rules/communicator.md`.

| Symptom | Start at |
|---|---|
| UI shows an error | MC frontend (browser console) |
| Status not updating | MC backend → umh-core outbound channel |
| Bridge stuck in "starting" | umh-core FSM → benthos-umh process log |
| Data not flowing | rendered benthos config → template expansion (`.claude/rules/config-templates.md`) |
| Process crash | benthos-umh log → S6 |

A symptom's location is rarely the cause's location. Example: an FSM error is often a benthos config validation failure.

## Service state

```bash
cd umh-core/tools/s6-analyzer && go build && ./s6-analyzer <service dir>
```

Look for `down` files blocking startup, missing `supervise` directories, and services whose directory exists while S6 returns nothing.

## Code paths

Find where a log line comes from (`grep -rn "<message>" umh-core/pkg/`), then trace through the reconcile loop to the condition that blocks the transition. Every stuck FSM has a trigger, a stuck state and a missing transition. Find all three.

## Known incident: templates lost from config.yaml

Seen with config sync from the browser (FileSystem API, polling about every 500 ms, `keepExistingData: false` overwrites) racing the backend config manager. The Go mutex does not cover browser writes. Template resolution clears `Templates` in the in-memory config (`convertYamlToSpec` in `pkg/config/yamlParsing.go`), so templates cannot be rebuilt from child instances. Check `grep -A5 "templates:" config.yaml` and orphaned `templateRef:` values. Restore the `templates:` section from a backup, and disable config sync while deploying.

## Writing up

Root cause in one sentence, evidence (log lines, config, S6 state), minimal reproduction, and why earlier fixes did not work.
