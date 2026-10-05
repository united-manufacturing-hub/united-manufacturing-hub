# Bridges do not start

A refused bridge stays pending and shows the reason in its status. This page explains what each reason means and what to do.

umh-core refuses a new bridge when one of the instance's resources is degraded, when its resource health is not proven yet, or when the instance already holds as many bridges as its CPU cores allow. The reason says which of the three it is. umh-core refuses bridges only while `agent.enableResourceLimitBlocking` is `true`.

## What the reason says

- **CPU degraded: [message]** — the instance's CPU is short on headroom. The message comes from the CPU health check. The [CPU Health](../../production/cpu-health.md) page explains every status and how to fix it.
- **Memory degraded: [message]** — memory is short. Free memory on the host, or raise the container's memory limit.
- **Disk degraded: [message]** — the disk is short. Free space on the volume, or make the volume larger.
- **Resource health not proven yet** — umh-core has not yet shown the instance's resources to be healthy. A bridge always waits for this proof, so a fresh instance starts no bridges until its first health readings arrive. If the reason stays at "no health reading yet", the instance cannot read its own resources: use the emergency setting below and report it to UMH. The reason "instance not active yet" means the container has not reached the active state; the same advice applies.
- **Cannot create bridge - limit exceeded (N bridges maximum with X CPU cores, 1 core reserved for Redpanda)** — the instance has reached its bridge limit. The count includes the bridges already running and the pending bridges listed before this one in config.yaml. The [Sizing Guide](../../production/sizing-guide.md) explains the limit and what raises it.

## What to do

1. Read the reason on the bridge's status.
2. Fix the resource it names. For CPU, follow the [CPU Health](../../production/cpu-health.md) page. For the bridge limit, follow the [Sizing Guide](../../production/sizing-guide.md).
3. Wait. The bridge starts on its own once the resource is healthy again, or once the instance runs fewer bridges than its limit. Nothing needs to be redeployed.

## Start bridges anyway in an emergency

Only if the bridge is needed now, turn the refusals off. This turns off all three, including the bridge limit. Edit the instance's [Config File](../instances/config-file.md):

```yaml
agent:
  enableResourceLimitBlocking: false
```

The setting takes effect without a restart. It is an emergency fallback, not a permanent configuration: with it off, nothing stops a new bridge from loading an instance that is already short on resources. Set it back to `true` once the resource problem is fixed.

## After a restart

After a restart, every bridge waits until the instance's resource health is proven. The first bridges in config.yaml order then start, up to the bridge limit. The rest stay pending with the limit reason, and each starts once the instance runs fewer bridges than its limit, for example after another bridge is removed.
