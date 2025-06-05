# Protocol‑Converter FSM / Manager – Developer Notes

## 1  Two config shapes and who owns them

| Go type & name                                        | Still contains `{{ }}`? | Who passes it?         | Who consumes it?                           |
| ----------------------------------------------------- | ----------------------- | ---------------------- | ------------------------------------------ |
| **Spec** – `ProtocolConverterServiceConfigSpec`       | **Yes** (user template) | Control‑loop → Manager | Manager ➜ *service* (`BuildRuntimeConfig`) |
| **Runtime** – `ProtocolConverterServiceConfigRuntime` | **No** – fully rendered | Manager → FSM          | FSM instance & `ProtocolConverterService`  |

---

## 2  How the manager must call the service

```go
// inside compareConfig / setConfig callbacks or before Add/Update
spec := cfg.ProtocolConverterServiceConfig // templated spec from YAML

runtimeCfg, err := pcService.BuildRuntimeConfig(
    &spec,                     // templated spec
    full.Agent.Location,       // authoritative agent.location map
    spec.Location,             // pc-specific location override (may be nil)
    globalVars,                // bundle prepared by central loop
    nodeName,                  // Kubernetes node the PC pod runs on
    cfg.Name,                  // logical PC name – becomes .internal.id
)
```

> **Why the manager supplies `nodeName`:** The service uses it to derive a
> stable `bridged_by` header (`protocol-converter-<node>-<pc>`).

### Manager responsibilities recap

1. **Receive** the *Spec* from `FullConfig`.
2. **Call** `BuildRuntimeConfig` **exactly once** per life‑cycle event (*add*, *update*).
3. **Pass** the returned *Runtime* struct to:

   * `AddToManager(ctx, fs, &runtimeCfg, pcName)`
   * `UpdateInManager(ctx, fs, &runtimeCfg, pcName)`
4. **Compare**: diff *Runtime* (not *Spec*) against the FSM instance copy.
5. Desired‑state transitions & rate‑limiting stay unchanged.

---

## 3  What `BuildRuntimeConfig` really does (for your mental model)

1. **Merge location**
   `agent.location` wins; PC may extend to higher indices; gaps are filled with `"unknown"`.
2. **Assemble variable bundle**
   `user` + `location` + `global` + `internal.id`.
3. **Calculate** `bridged_by`
   `protocol-converter-<sanitised‑node>-<pc>`
4. **Render** the three sub‑templates with `text/template`.
5. **Enforce** UNS guard‑rails (read‑DFC output & write‑DFC input).

If any `{{` survive step 4, the helper returns an error and the manager must abort the update.

---

## 4  Quick code sketch – updated

```go
// compareConfig callback
func(inst fsm.FSMInstance, cfg config.ProtocolConverterConfig) (bool, error) {
    pcInst := inst.(*ProtocolConverterInstance)

    rt, err := pcService.BuildRuntimeConfig(
        &cfg.ProtocolConverterServiceConfig,
        fc.Agent.Location,
        cfg.ProtocolConverterServiceConfig.Location,
        globalVars,
        nodeName,
        cfg.Name,
    )
    if err != nil {
        return false, err // propagate upwards – manager will handle
    }

    return pcInst.RuntimeCfg.Equal(rt), nil
}
```
