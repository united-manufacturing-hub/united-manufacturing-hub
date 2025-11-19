# Sizing Guide

**Start with → 2 vCPU · 4 GB RAM · 40 GB SSD**

#### What that box handles

* **≈ 4 bridges instances** (e.g. OPC UA ➜ Redpanda) **plus one bridge instance** that forwards from the local Redpanda to an external MQTT broker
* **≈ 900 tags at 1 message / second each**
* Keeps **seven days** of history under the default cluster retention (`log_retention_ms = 7 days`)
* Runs comfortably below 70% CPU (automatic throttling protection kicks in above this)

#### Disk usage in practice

Redpanda writes **128 MiB segments**; a segment can be deleted only after it is closed.\
With Snappy compression, a typical 200 B JSON payload shrinks to ≈ 50–70 B (3–4× ratio).\
Allowing a 5 GB safety buffer, a 40 GB SSD gives **≈ 35 GB usable history ≙ ~500–700 million messages**.

_Need more?_\
Shorten retention (either during install with `internal.redpanda.redpandaServiceConfig.defaultTopicRetentionMs` or later on the topic level using `rpk`) or enlarge the disk.

#### Memory

| Component           | Rule of thumb                                    |
| ------------------- | ------------------------------------------------ |
| Redpanda            | ≈ 2 GB · cores + 1.5 GB head-room (Seastar rule) |
| Agent + supervision | ≈ 150 MB                                         |
| Each extra pipeline | ≈ 100 MB                                         |

#### CPU

_One core_ is reserved for Redpanda to ensure reliable message processing. Additional cores are allocated for protocol converter bridges and data processing.

**Theoretical Bridge Limits:**
- We recommend **5 bridges per CPU core** (after reserving 1 core for Redpanda)
- Example: 2 CPU cores = (2-1) × 5 = **5 bridges maximum**
- Example: 4 CPU cores = (4-1) × 5 = **15 bridges maximum**

**Dynamic Resource Protection:**
Since every bridge has different resource requirements (OPC UA with 10,000 tags uses more CPU than MQTT with 100 tags), we also monitor actual resource usage:

- **CPU Utilization**: Blocks new bridges if CPU usage exceeds 70%
- **CPU Throttling**: Blocks if the container is being throttled. Throttling means the system needs brief CPU bursts (e.g., when processing message batches) but hits the CPU limit, causing delays and degraded performance even if average CPU usage looks acceptable
- **Memory Usage**: Blocks if memory exceeds 80%
- **Disk Usage**: Blocks if disk exceeds 85%

**Automatic Enforcement:**
The system will prevent you from deploying new bridges if:
1. You've reached the theoretical limit for your CPU allocation, OR
2. The system detects resource degradation (high CPU, throttling, memory, or disk pressure)

This resource-based blocking is controlled by a feature flag and can be configured in your `config.yaml`:
```yaml
agent:
  enableResourceLimitBlocking: false  # Disable resource-based bridge blocking (default: true)
```

When enabled, this ensures system stability and prevents one bridge from impacting others. If you need more bridges, either:
- Increase CPU allocation (for containerized deployments)
- Upgrade to a larger instance (for VM/bare-metal deployments)
- Optimize existing bridges (reduce polling rates, tag counts, or processing complexity)

#### Resource Limit Error Messages

When the system blocks bridge creation, you'll see clear messages explaining why:

- **Bridge limit**: `Cannot create bridge - limit exceeded (5 bridges maximum with 2.0 CPU cores, 1 core reserved for Redpanda)`
- **CPU throttling**: `CPU throttled (15% of time). Container limited to 2.0 cores, needs more during peaks (host has 8 cores available)`
- **High CPU**: `CPU degraded: CPU utilization critical`
- **High Memory**: `Memory degraded: Memory usage at 85%`
- **High Disk**: `Disk degraded: Disk usage at 90%`

#### Easy vertical scaling

UMH Core is stateless besides the **`/data`** volume. To grow:

1. Stop the container
2. Move or resize the volume / attach it to a bigger VM
3. Start the same image — no reinstall or re-configuration required
