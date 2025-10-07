# UI/UX standards

## Appendix F: User Experience Requirements

### Core Philosophy
**Opinionated software that grows with users. We make decisions so users can focus on their business.**

Like Apple, Linear, and Figma, we believe in fewer options that work perfectly rather than many options that might work. We build for the electrical engineer configuring PLCs, the process engineer analyzing production data, the typical historian user, AND the power user automating 200 machines. Our open-source model forces us to create software people actually want to use, not just buy.

### Four Pillars of UX

#### 1. Immediate Trust
**Every interaction builds confidence through instant feedback and transparency.**

Users abandon silent operations, which is why every operation must emit a status update at least every 500ms—even if it's just "Still processing (15s elapsed)." The first feedback must arrive within 100ms of any action because modern users interpret no feedback as a broken UI.

We show real values like "Updating pump-03 flow: 50→75 L/min" instead of generic "Updating configuration" messages because engineers need to verify they're operating on the correct equipment. Progress indicators must be honest—if we don't know the percentage, we show elapsed time instead of fake progress bars that sit at 90% for minutes.

Operations can be hidden or minimized because real engineering work requires multi-tasking. An electrical engineer shouldn't have to babysit a PLC configuration update—they should be able to start it, minimize it, and check sensor mappings while it runs.

#### 2. Opinionated Simplicity
**One way that works, not ten ways that might work.**

There is exactly one way to accomplish each task in UMH. No alternative workflows, no preference dialogs, no "choose your method." This isn't limiting—it's liberating. Process engineers don't want to evaluate three different ways to connect to a historian; they want the one way that works reliably.

The system auto-detects everything it can—ports, protocols, data types. Configuration is procrastination. If 95% of users need the same buffer size, that's the only buffer size. Safe defaults mean destructive operations are isolated and require explicit confirmation. If an engineer does nothing, nothing breaks.

We enforce tested limits to prevent production disasters. If we've tested the system with 10,000 tags per bridge on 2 CPU cores, the system prevents creating a bridge with 15,000 tags. The error message is clear: "Cannot exceed tested configuration of 10,000 tags. Upgrade to 4 CPU cores or reduce tag count." Better to stop than crash at 3 AM.

Beta features live behind feature flags, clearly marked and opt-in only. Developers shouldn't fear shipping early as long as users explicitly choose to try experimental features. This lets us learn quickly while keeping production systems stable.

#### 3. Progressive Power
**Start with clicks, graduate to code, never hit a ceiling.**

Every UI interaction generates readable, reusable YAML that users can inspect, learn from, and eventually modify. An electrical engineer clicks "Add Modbus Device" in week one and sees the generated configuration. By month one, they're copying and modifying that YAML for similar devices. By year one, they have a Git repository with templated configurations for their entire plant.

The reverse is equally true—hand-written YAML renders perfectly in the UI. There's no penalty for becoming a power user, no features locked behind the graphical interface. This isn't just about convenience; it's about growth. The same system that welcomed a process engineer on day one scales to their expertise on year ten.

All configuration lives in version-controlled files, not hidden databases. This enables real DevOps workflows—diff your changes, collaborate through pull requests, roll back mistakes. Your configurations work with ChatGPT, Copilot, or whatever AI tool emerges next because it's just clean, standard YAML.

Most importantly, there's always an escape hatch. If our UI doesn't support your edge case, drop to YAML. If YAML isn't flexible enough, script it. If you need something we haven't built yet, use kubectl directly. You're never blocked waiting for us to add a feature.

#### 4. Error Excellence
**Every error builds trust through clarity and recovery.**

Errors come in two layers because different users need different information. The process engineer sees "Cannot connect to PLC - The device at 192.168.1.100 isn't responding. Check if it's powered on and connected to the network." The power user can expand to see "ECONNREFUSED 192.168.1.100:502 (modbus/tcp), last success 5 minutes ago, retry in 30s."

Every error includes actionable next steps. Never just "Connection failed"—always "Connection failed. Check if the device is powered on and connected to the network." We include context like last successful connection time and when we'll retry because users need situational awareness to make decisions.

We never blame the user. There's no such thing as "user error" in our messages. If the UI allowed it, it should work. If it doesn't work, we explain what happened and how to recover, preserving the user's dignity and confidence.

All operations are reversible. Every configuration change can be rolled back, every deployment can be undone. Mistakes happen—especially when an electrical engineer is troubleshooting at 2 AM—and they shouldn't be permanent. The rollback path is always clear and always works.

### The Design Hierarchy

When principles conflict, we prioritize in this order: **Safety** (production never stops), **Clarity** (obvious beats clever), **Simplicity** (minimum cognitive load), **Power** (advanced capability when needed), and finally **Flexibility** (only after all others are satisfied).

The UMH is opinionated software that respects both the process engineer who needs reliability and the power user who needs control. We make the hard decisions so our users can focus on theirs.
