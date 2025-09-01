# Sizing Guide

**Start with → 2 vCPU · 4 GB RAM · 40 GB SSD**

#### What that box handles

* **≈ 9 bridges instances** (e.g. OPC UA ➜ Redpanda) **plus one bridge instance** that forwards from the local Redpanda to an external MQTT broker
* **≈ 900 tags at 1 message / second each**
* Keeps **seven days** of history under the default cluster retention (`log_retention_ms = 7 days`)
* Runs comfortably below 70 % CPU and I/O on a Hetzner CAX21 / CX32-class VM or Raspberry Pi 4

#### Disk usage in practice

Redpanda writes **128 MiB segments**; a segment can be deleted only after it is closed.\
With Snappy compression, a typical 200 B JSON payload shrinks to ≈ 20 B.\
Allowing a 5 GB safety buffer, a 40 GB SSD gives **≈ 35 GB usable history ≙ \~2.8 billion messages**.

_Need more?_\
Shorten retention (either during install with `internal.redpanda.redpandaServiceConfig.defaultTopicRetentionMs` or later on the topic level using `rpk`) or enlarge the disk.

#### Memory

| Component           | Rule of thumb                                    |
| ------------------- | ------------------------------------------------ |
| Redpanda            | ≈ 2 GB · cores + 1.5 GB head-room (Seastar rule) |
| Agent + supervision | ≈ 150 MB                                         |
| Each extra pipeline | ≈ 100 MB                                         |

#### CPU

_One core_ is kept busy by Redpanda. _One additional core_ comfortably covers the agent and the first dozen pipelines doing light transforms. Heavy parsing, encryption, or synchronous HTTP calls may warrant more cores or a faster CPU.

##### Performance benchmarks

Based on testing with an AMD Ryzen 9 7950X 16-Core Processor with 4GB allocated memory:

| CPU Cores | Bridge Instances |
|-----------|------------------|
| 2         | ~10 bridges      |
| 4         | ~20 bridges      |
| 6         | ~30 bridges      |
| 8         | ~35 bridges      |

Additional CPU cores beyond 8 show diminishing returns for bridge capacity.
Currently horizontal scaling is recommended for more than 35 bridges.

#### Easy vertical scaling

UMH Core is stateless besides the **`/data`** volume. To grow:

1. Stop the container
2. Move or resize the volume / attach it to a bigger VM
3. Start the same image — no reinstall or re-configuration required
