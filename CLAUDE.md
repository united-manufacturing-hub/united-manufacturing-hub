# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

The United Manufacturing Hub (UMH) is an Industrial IoT platform for manufacturing data ingestion and management. It has two main components:

1. **UMH Core** (`umh-core/`) - Modern single-container edge gateway
2. **UMH Classic** (`deployment/united-manufacturing-hub/`) - Full Kubernetes deployment with Helm charts

## Terminology

- **Bridge** (UI) = `protocolConverter:` (YAML) = Protocol Converter (legacy)
- **Stand-alone Flow** (UI) = `dataFlow:` (YAML) = Data Flow Component/DFC (legacy)
- **Stream Processor** = `dataFlow:` with `sources:[]` array (aggregates multiple topics)
- **Data Contract** = underscore-prefixed type (`_raw`, `_pump_v1`, `_maintenance_v1`)
- **Virtual Path** = optional organizational segments in topics (e.g., `motor.electrical`)
- **Tag** = single data point/sensor (industrial term)
- **_raw** → **_devicemodel_v1** → **_businessmodel_v1** (data progression)

## Non-Intuitive Patterns

- **Variable flattening**: `variables.IP` → `{{ .IP }}` (nested becomes top-level)
- **S6 logs**: `.s` = clean rotation, `.u` = unfinished (container killed), `current` = active log
- **Empty FSMState**: `''` means S6 returns nothing (directory missing/corrupted)
- **FSM precedence**: Lifecycle states ALWAYS override operational states
- **One tag, one topic**: Never combine sensors in one payload (avoids timing/merge issues)
- **Bridge = Connection + Source Flow + Sink Flow**: Connection only monitors network availability
- **Data validation**: Happens at UNS output plugin, not at source
- **Bridge states**: `starting_failed_dfc_missing` = no data flow configured yet
- **Resource limiting**: Controlled by `agent.enableResourceLimitBlocking` and related settings. Default: ≤70% CPU; ~5 bridges per CPU core after reserving 1 for Redpanda
- **Template variables**: `{{ .IP }}`, `{{ .PORT }}` auto-injected from Connection config
- **Location computation**: Agent location + bridge location = `{{ .location_path }}`

## Essential Commands

**Build**: `make build` (standard), `make build-debug` (debug), `make build-pprof` (profiling)

**Test**: `make test` (all), `make unit-test`, `make integration-test`, `make benchmark`

**Dev**: `make test-graphql` (port 8090), `make pod-shell`, `make test-no-copy` (use current config)

**Clean** (destructive): `make stop-all-pods`, `make cleanup-all`

**MUST run before completing tasks**: `golangci-lint run`, `go vet ./...`, check no focused tests with `ginkgo -r --fail-on-focused`

**Git**: Default branch is `staging`. Lefthook runs gofmt, go vet, license checks on commit; nilaway, golangci-lint on push.

## Architecture & Key Decisions

### System Architecture

```
┌─────────────────────┐
│   Agent (Go)        │ ← Reads /data/config.yaml, manages lifecycle
├─────────────────────┤
│   FSM Controllers   │ ← State machines for each component
│   - Benthos FSM     │   
│   - Redpanda FSM    │   
│   - S6 FSM          │   
├─────────────────────┤
│   Service Layer     │
│   - Benthos-UMH     │ ← Stream processing (Data Flow Components)
│   - Redpanda        │ ← Kafka-compatible message broker
│   - GraphQL API     │ ← Data access API
└─────────────────────┘
```

### FSM Pattern

**Separation of Concerns**:
- `machine.go`: FSM definition with state constants and transitions
- `fsm_callbacks.go`: Fail-free callback implementations (logging only, no errors)
- `actions.go`: Idempotent operations with context handling (can fail and retry)
- `reconcile.go`: Single-threaded control loop (only place that modifies state)
- `models.go`: Data structures and types

**State precedence**: Lifecycle (`to_be_created`, `removing`) > Operational (`running`, `stopped`)

**Key rules**:
- Actions must be idempotent (will retry on failure)
- Only reconciliation loop modifies state (deterministic, single-threaded)
- FSM callbacks fail-free (logging only, never return errors)
- Use exponential backoff for failed transitions
- All actions must handle context cancellation

### Data Architecture Decisions

**Two-layer model**:
- **Device models** (`_pump_v1`): Equipment internals, sites control
- **Business models** (`_maintenance_v1`): Enterprise KPIs, aggregated views

**UNS principles**:
- **Publish regardless**: Producers don't wait for consumers
- **Entry/exit via bridges**: All data validated at gateway
- **Location path**: WHERE in organization (enterprise.site.area)
- **Device model**: WHAT data exists (temperature, pressure)
- **Virtual path**: HOW to organize within model (motor.electrical)

## Testing Guidelines

- **Framework**: Ginkgo v2 with Gomega matchers
- **Integration Tests**: Use Testcontainers for Docker-based testing
- **Unit Tests**: Focus on business logic, avoid mocking FSM internals
- **Key Test Patterns**:
  ```go
  // Use focused specs during development (CI fails if any are present)
  // CI runs with: ginkgo -r -p --fail-on-focused
  FIt("should handle state transition", func() {
      // Test implementation
  })

  // Test FSM transitions explicitly
  Eventually(func() State {
      return fsm.Current()
  }).Should(Equal(StateRunning))
  ```

## Code Style Requirements

1. **Do NOT add comments unless explicitly requested**
2. **Prefer editing existing files over creating new ones**
3. **Follow existing patterns in the codebase**
4. **Struct field alignment**: Order fields by decreasing size
5. **Error handling**: Return errors up the stack, handle in reconciliation loop
6. **No direct FSM state changes**: Always go through reconciliation

## Go Performance Patterns

### Object Pooling

Use `sync.Pool` to reduce GC pressure for frequently allocated objects:

```go
var bufferPool = sync.Pool{
    New: func() interface{} {
        return new(bytes.Buffer)
    },
}

func processData(data []byte) {
    buf := bufferPool.Get().(*bytes.Buffer)
    defer func() {
        buf.Reset()
        bufferPool.Put(buf)
    }()
    buf.Write(data)
    // Process buffer
}
```

**When to use**: Objects allocated/freed in hot paths (>1000/sec), especially large objects.

### Memory Preallocation

Preallocate slices and maps when size is known:

```go
// Good: Preallocate with known capacity
items := make([]Item, 0, expectedSize)
cache := make(map[string]Value, expectedSize)

// Avoid: Growing dynamically causes multiple allocations
items := []Item{}  // Will reallocate as it grows
```

**Rule of thumb**: If you know approximate size, preallocate. Saves 3-5 allocations per slice growth.

### Struct Field Alignment

Order struct fields by decreasing size to minimize padding:

```go
// Bad: 32 bytes with padding
type BadStruct struct {
    flag bool      // 1 byte + 7 bytes padding
    count int64    // 8 bytes
    id int32       // 4 bytes + 4 bytes padding
}

// Good: 16 bytes, no padding
type GoodStruct struct {
    count int64    // 8 bytes
    id int32       // 4 bytes
    flag bool      // 1 byte + 3 bytes padding (at end)
}
```

**Impact**: Can reduce struct size by 30-50% in many cases.

### Zero-Copy Techniques

Avoid unnecessary copies, especially for large data:

```go
// Bad: Creates copy
func ProcessData(data []byte) {
    dataCopy := make([]byte, len(data))
    copy(dataCopy, data)
    // Process dataCopy
}

// Good: Use slices to reference original data
func ProcessData(data []byte) {
    // Process data directly (read-only)
    // Or use subslices: segment := data[offset:offset+length]
}
```

**When safe**: If function doesn't need to modify data and data lifetime is longer than function execution.

### Stack vs Heap Allocations

Keep allocations on stack when possible (escape analysis):

```go
// Bad: Escapes to heap (pointer returned)
func createConfig() *Config {
    cfg := Config{...}
    return &cfg  // cfg escapes to heap
}

// Good: Stays on stack (value returned)
func createConfig() Config {
    return Config{...}  // Allocated on stack
}
```

**Check with**: `go build -gcflags="-m"` to see escape analysis.

### Goroutine Worker Pools

Limit goroutine count with worker pools for CPU-bound tasks:

```go
func processItems(items []Item) {
    numWorkers := runtime.NumCPU()
    workCh := make(chan Item, numWorkers)

    // Start workers
    var wg sync.WaitGroup
    for i := 0; i < numWorkers; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for item := range workCh {
                process(item)
            }
        }()
    }

    // Send work
    for _, item := range items {
        workCh <- item
    }
    close(workCh)
    wg.Wait()
}
```

**Avoid**: Creating unbounded goroutines (e.g., `for _, item := range items { go process(item) }`).

### Atomic Operations

Use atomics for lock-free counters and flags:

```go
import "sync/atomic"

type Counter struct {
    count atomic.Int64
}

func (c *Counter) Increment() {
    c.count.Add(1)
}

func (c *Counter) Get() int64 {
    return c.count.Load()
}
```

**When to use**: Simple counters, flags, or pointers accessed from multiple goroutines.

### Lazy Initialization

Defer expensive initialization until first use:

```go
import "sync"

type Service struct {
    clientOnce sync.Once
    client     *ExpensiveClient
}

func (s *Service) getClient() *ExpensiveClient {
    s.clientOnce.Do(func() {
        s.client = newExpensiveClient()
    })
    return s.client
}
```

**When to use**: Optional features, expensive clients used in <50% of requests.

### Immutable Data Sharing

Share read-only data across goroutines without locks:

```go
// Safe: Config is read-only after initialization
type Config struct {
    MaxConnections int
    Timeout        time.Duration
}

var globalConfig atomic.Pointer[Config]

func updateConfig(newConfig Config) {
    globalConfig.Store(&newConfig)  // Atomic pointer update
}

func getConfig() *Config {
    return globalConfig.Load()  // Safe concurrent reads
}
```

**When to use**: Configuration, lookup tables, caches that update infrequently.

### Context Management

Always pass context and check cancellation in long operations:

```go
func processWithContext(ctx context.Context, items []Item) error {
    for _, item := range items {
        select {
        case <-ctx.Done():
            return ctx.Err()  // Respect cancellation
        default:
            if err := process(item); err != nil {
                return err
            }
        }
    }
    return nil
}
```

**Critical**: All FSM actions must handle context cancellation.

### Efficient Buffering

Use buffered channels to reduce synchronization overhead:

```go
// Bad: Unbuffered channel blocks on every send
ch := make(chan Message)

// Good: Buffered channel allows burst sends
ch := make(chan Message, 100)
```

**Rule**: Buffer size ≈ expected burst size or 2× number of producers.

### Batching Operations

Batch small operations to reduce overhead:

```go
// Bad: Write each message individually
for _, msg := range messages {
    kafka.Write(msg)  // Network round-trip per message
}

// Good: Batch writes
batch := make([]Message, 0, len(messages))
for _, msg := range messages {
    batch = append(batch, msg)
    if len(batch) >= 100 {
        kafka.WriteBatch(batch)
        batch = batch[:0]
    }
}
if len(batch) > 0 {
    kafka.WriteBatch(batch)  // Write remaining
}
```

**When to use**: Network I/O, database writes, file operations.

## Documentation Maintenance

### Core Principle

**No code change without corresponding documentation updates.**

Every code modification must include relevant documentation changes in the same PR. This ensures documentation stays synchronized with implementation and reduces technical debt.

### Documentation Location Mapping

**UMH Core (`umh-core/`)**:
- **Architecture changes** → `docs/architecture/`
- **FSM modifications** → `docs/fsm/`
- **API endpoints** → `docs/api/`
- **Configuration options** → `docs/configuration/`
- **Deployment guides** → `docs/deployment/`

**Features/Components**:
- **New features** → Add to `docs/features/` with examples
- **Protocol converters** → Update `docs/bridges/`
- **Data flow components** → Update `docs/dataflows/`
- **Stream processors** → Update `docs/processors/`

**Breaking Changes**:
- **Version upgrade guides** → `docs/migration/`
- **Deprecation notices** → Mark in relevant docs + `CHANGELOG.md`
- **Configuration changes** → Update examples in `config/` directory

### Documentation Requirements by Change Type

**Code Changes**:
- Public API: Update function documentation (godoc)
- Configuration: Update YAML examples and schema docs
- FSM states: Update state machine diagrams
- Error handling: Document new error codes/messages

**Bug Fixes**:
- If fix changes behavior: Update relevant user-facing documentation
- If fix is internal: Update architecture/implementation docs
- Always: Add to `CHANGELOG.md` under "Bug Fixes"

**New Features**:
- Feature documentation in `docs/features/`
- Configuration examples in `config/examples/`
- Update main `README.md` if user-facing
- Add to `CHANGELOG.md` under "New Features"

### Documentation Update Checklist

Before marking PR as ready for review:
- [ ] Code changes have corresponding doc updates
- [ ] Examples tested and verified
- [ ] CHANGELOG.md updated
- [ ] Breaking changes clearly documented
- [ ] Migration guides provided (if needed)
- [ ] API documentation regenerated (if applicable)

### Tools and Validation

**Generate docs**:
```bash
make generate  # Regenerates GraphQL schema docs, OpenAPI specs
```

**Validate docs**:
```bash
make lint-docs  # Check for broken links, formatting issues
```

**Local preview**:
```bash
make serve-docs  # Start local documentation server
```

### Anti-Patterns to Avoid

**Don't**:
- Defer documentation to "later" (it never happens)
- Write documentation separately from code changes
- Assume "the code is self-documenting"
- Leave TODO comments in documentation
- Create documentation debt intentionally

**Do**:
- Write docs alongside code in same commit/PR
- Update examples when changing behavior
- Remove outdated documentation immediately
- Keep CHANGELOG.md up to date with every PR

### Documentation Review Guidelines

When reviewing PRs:
1. **Check completeness**: Are all user-facing changes documented?
2. **Verify accuracy**: Do examples actually work?
3. **Test migration paths**: Can users upgrade without breaking changes?
4. **Review clarity**: Is documentation clear for target audience?

## Support & Troubleshooting Workflows

This section covers workflows for investigating and resolving production issues in UMH Core deployments.

### Linear/Sentry Integration

#### Critical Linear Integration Rules

**NEVER modify issue title or description unless explicitly requested by the user**
- Screenshots and original content in Linear tickets must be preserved
- Use comments for analysis, insights, and resolution documentation
- Only update status/labels/assignee fields when appropriate

#### Linear Investigation Protocol

**Always start with comments** - They contain the real story, not the description:

1. **Comments BEFORE description**:
   ```bash
   # FIRST THING - before even looking at description
   mcp__linear-server__list_comments issueId: "ISSUE-ID"
   ```

2. **Fetch ALL comments immediately** - Critical details are often in updates, not the description
3. **Download ALL attachments** - Screenshots, logs, and files contain evidence
4. **Transcribe screenshots immediately** - Don't wait, evidence disappears
5. **Check for related issues** - Search for duplicates, dependencies, and similar problems
6. **Identify existing workarounds** - Users often document their current painful solutions
7. **Look for patterns across issues** - Multiple related issues may need a parent epic
8. **Quantify customer impact** - "100 processors instead of 1" is clearer than "performance issues"
9. **Create parent epics when needed** - Group related issues under strategic initiatives
10. **Check if already fixed** - Search recent PRs, the fix might already exist

**Key Questions to Answer**:
- What are customers doing TODAY to work around this? (check comments!)
- How many customers affected? Named examples?
- Is this a symptom of a larger architectural gap?
- Are there related issues that should be solved together?
- Has this been fixed in another issue/PR already?

**Common Pitfalls**:
- Assuming the issue description is complete (it never is)
- Missing critical context in comment threads
- Not recognizing when multiple issues are the same root cause
- Over-engineering solutions instead of simple foundations
- Not checking if it's already been fixed elsewhere

#### Linking Linear and Sentry Issues

1. **In Linear**: Add Sentry URLs as attachments to ticket
2. **In Sentry**: Navigate to issue → "Linked Issues" panel → Link to Linear ticket
   - **Do NOT use MCP tools** - Ask user to link manually in Sentry UI
   - This enables automatic reopening when errors recur

#### Incident Response Comment Template

```markdown
## 🔍 Root Cause Analysis

### Key Evidence
- [Sentry Issue ID](url): Description
- Log Location: `path/to/log:line_number`
- Critical Error: [timestamp] "exact error message"

### Timeline
[Detailed timeline with evidence]

### Technical Analysis
[Root cause explanation]

### Recommendations
**Immediate**: [Quick fixes]
**Long-term**: [Permanent solutions]
```

#### GitHub Actions Investigation

**IMPORTANT**: Must be run from within git repository directory. If you get "failed to determine base repo", navigate to the repository first.

```bash
# Basic commands
gh run list --limit 20
gh run view <run-id> --log-failed
gh run view <run-id> --job <job-id> --log

# Extract raw logs when run is still in progress
curl -sL "https://api.github.com/repos/united-manufacturing-hub/united-manufacturing-hub/actions/jobs/<job-id>/logs" \
  -H "Authorization: token $(gh auth token)" > job_raw.txt

# Parse the raw logs (contains escape sequences)
cat job_raw.txt | sed 's/\\r\\n/\n/g' | sed 's/\\t/    /g' | grep -A 100 "pattern"
```

**Data Race Detection**:
```bash
# Go test with race detector (enabled in CI by default)
go test -race ./...

# Extract race warnings from CI logs
gh run view <run-id> --job <job-id> --log | grep -A 100 "WARNING: DATA RACE"

# Key indicators of data races:
# - "testing.go:1490: race detected during execution of test"
# - "WARNING: DATA RACE" followed by stack traces
# - Multiple goroutines writing to same memory address
# - Look for concurrent operations in stack traces (e.g., dialParallel)
```

**CI Failure Investigation Workflow**:

1. Check if failure is related to PR:
   ```bash
   gh pr diff <pr-number> --name-only  # List changed files
   ```

2. Check base branch history:
   ```bash
   gh run list --branch staging --limit 10 --json conclusion,createdAt
   # Look for recent successful runs to confirm issue is new
   ```

3. For unrelated test failures:
   - Document the issue with evidence
   - Create Linear ticket but don't block PR
   - Check if another PR already addresses it
   - Assign to developer working in that area

**Linear Ticket Best Practices for Test Failures**:
- **Title**: Include test suite and brief description
- **Evidence**: Link to failed CI run, include relevant stack traces
- **Cross-reference**: Link related PRs (both failing and fixing)
- **Assignment**: Assign to developer already working in affected code
- **Priority**: Set appropriately (test-only races = Low/Normal)
- **Note solutions**: If another PR fixes it, mention in description

### Instance Offline Troubleshooting

This workflow handles cases where instances appear offline in Management Console but may actually be running.

#### 1. Immediate Actions

```bash
# Download and extract customer logs (usually 7z archives)
brew install p7zip  # If needed
7z x "*.7z"

# Search for critical patterns in logs
grep -E "Outbound message channel is full|Heartbeat push send a warning|connection reset by peer|context deadline exceeded|Failed to generate status message" logs/*.s
```

#### 2. Critical Error Patterns

| Pattern | Meaning | Location |
|---------|---------|----------|
| `Outbound message channel is full` | Communicator queue overflow | `pkg/communicator/api/v2/push/push.go:89` |
| `Heartbeat push send a warning` | Progressive connection degradation | `pkg/watchdog/watchdog.go:318` |
| `connection reset by peer` | Network path issues with Cloudflare | `pkg/communicator/api/v2/http/requester.go:223` |
| `context deadline exceeded` | Timeout issues | Various FSM components |
| `Failed to generate status message` | Communicator failures | `pkg/subscriber/subscribers.go:144` |

#### 3. Sentry Investigation

Search strategies (in order of effectiveness):
1. By customer location/region (limited customers per region)
2. By error message patterns (not geo.region tags)
3. By issue IDs: UMH-CORE-7D (EOF), UMH-CORE-GP (TCP reset), UMH-CORE-BG (404s)

#### 4. Timeline Creation Template

**Build timeline DURING investigation, not after:**

```
[Timestamp UTC] - [Source: Log/Sentry/Linear] - [Component/Event]
"Exact error message quote" OR "Event description"
Location: filename:line_number OR PR/Issue link
Evidence: [Link to Sentry issue or log location]

Example:
[2025-07-30] - Linear ENG-3293 - Port 9000 conflict reported
[2025-08-11] - PR #2195 merged - OS port allocation fix
[2025-08-26] - Linear ENG-3380 - Port 9002 conflict (issue persists)
[2025-09-08] - PR #2246 merged - BenthosMonitor GetConfig fix (actual resolution)
```

#### 5. Root Cause Patterns

**Instance shows offline but is running:**
- Network instability between customer and Cloudflare edge
- Message queue overflow in communicator
- MTU/packet fragmentation issues
- NAT/firewall state timeouts

**Why restarts work:**
- Clears message queue
- Resets TCP connection states
- May route through different Cloudflare edge

### Support Ticket Routing

Route issues to appropriate teams based on symptom classification:

#### IT/Support Team
- Instance offline/connectivity issues
- Network problems, DNS issues
- Infrastructure failures
- Customer-reported outages

#### Engineering Team (ENG)
- Code bugs, FSM failures
- Build/dependency failures
- Data flow component errors
- Test failures

**Priority Guidelines**: Never set automated issues to "Urgent". Use "High" for blocking issues.

### Investigation Best Practices

#### Do's
- Start with Linear ticket for context
- **Always fetch Linear comments FIRST** - they contain the real story, not the description
- **Transcribe all screenshots immediately** - crucial evidence often hidden in UI
- Download ALL log archives
- Search by customer location in Sentry (limited customers per region)
- **Create timeline WHILE investigating** - not after
- Include exact timestamps, quotes, and line numbers
- Check code paths for retry/backoff logic
- **Trust user hypotheses** - "maybe two windows?" often correct
- **Read screenshots carefully** - German text may contain key details
- **Follow data lifecycle** - Where created → stored → deleted → reconstructed
- **Check if already fixed** - Search recent PRs with broad terms
- **Verify status vs reality** - UI may show errors while service works fine

#### Don'ts
- Don't assume geo.region tagging is complete
- Don't trust Linear screenshot URLs (often need auth)
- Don't skip log timezone conversion to UTC
- Don't wait until end to document findings
- Don't trust issue descriptions alone - comments have updates
- Don't stop at first explanation - ask "why does THAT happen?"

#### Key Questions
- "What's the exact error pattern?"
- "When did it start vs when was it reported?"
- "How many instances affected?" (Check Sentry occurrences)
- "Why does restart fix it?" (Usually state/queue issues)
- "Was config sync running?" (Check for race conditions)
- "Were there multiple windows/tabs open?" (Multi-process conflicts)

### Efficient Investigation Tool Chain

#### Parallel Information Gathering

Always run these in parallel at the start:

```bash
# Linear context (run all together)
mcp__linear-server__get_issue id: "ISSUE-ID"
mcp__linear-server__list_comments issueId: "ISSUE-ID"  # CRITICAL - has real story
mcp__linear-server__list_issues query: "similar symptoms"

# GitHub search (broad terms work better)
gh pr list --limit 30 --state all --search "error message keywords"
gh pr list --limit 20 --state merged --search "component name"
```

#### Cross-Reference Workflow

1. **Search broadly first** - Don't limit to exact component
2. **Check time windows** - PRs merged between "worked" and "broken"
3. **Look for parent issues** - Many issues are children of bigger problems
4. **Check "Done" issues** - Your issue might already be fixed

#### GitHub PR Search Strategies

```bash
# Find recent changes to component
gh pr list --limit 30 --search "FSM" --state merged --json number,title,mergedAt

# Find fixes in time window
gh pr list --limit 50 --state merged --json mergedAt,title | jq '.[] | select(.mergedAt > "2025-08-20")'

# Search by error patterns
gh pr list --search "timestamp" --state all
gh pr list --search "Starting state" --state all

# Search for related fixes (run when debugging specific issues)
gh pr list --search "monitor GetConfig" --state all
gh pr list --search "port allocation" --state all
gh pr list --search "address already in use" --state all
gh pr list --search "<component-name> monitor" --state all
```

#### The "Already Fixed" Check

Before deep diving, always check if it's already fixed:
1. Search PRs with issue symptoms (not just issue ID)
2. Check recently merged PRs in affected components
3. Look for parent/related issues marked "Done"
4. Search by error message, not component name

### Race Condition Patterns

#### Multi-Process Config Modifications

**Problem**: Browser FileSystem API + Backend config manager = data loss

**Evidence**:
- Config sync polling (500ms default)
- `keepExistingData: false` = complete overwrite
- Go mutex doesn't protect against browser writes

**Detection**:
```bash
# Find rapid config writes
grep "Successfully wrote config" logs/*.txt | \
  awk '{print $1}' | uniq -c | sort -rn

# Check for concurrent operations
grep -B2 -A2 "deploy.*processor\|GetConfigFile" logs/*.txt
```

#### The Templates Deletion Bug

**Code Location**: `pkg/config/yamlParsing.go:151`
```go
processedConfig.Templates = TemplatesConfig{}  // DATA LOSS!
```

**Why it fails**: Templates deleted from memory, can't reconstruct from child instances

**Trigger**: Any config write when only child instances exist (no roots)

**Investigation**:
```bash
# Check if templates exist
grep -A5 "templates:" config.yaml

# Find orphaned template references
grep "templateRef:" config.yaml | cut -d: -f2 | sort -u

# Check for race condition evidence
grep -E "config sync|GetConfigFile|deploy-streamprocessor" logs/*.txt
```

**Fix**: Restore templates section manually or from backup

**Prevention**: Disable config sync when deploying components

## UMH Ecosystem Integration

This section explains how umh-core integrates with ManagementConsole and benthos-umh, and how to debug issues that span multiple repositories.

### Repository Overview

The UMH ecosystem consists of three interconnected repositories:

**umh-core** (this repository):
- Single-container edge gateway running on customer sites
- FSM-based component lifecycle management
- Communicator for bidirectional messaging with Management Console
- S6 supervision for process management
- Template expansion and benthos config generation
- GraphQL API for local data access

**ManagementConsole**:
- `frontend/`: Svelte 5 web application (user interface)
- `backend/`: Go API server (handles user requests, manages Redis message queues)
- Provides remote configuration and monitoring of umh-core instances
- Does NOT directly access umh-core - all communication via message queues

**benthos-umh**:
- Fork of Benthos stream processor with UMH-specific plugins
- Orchestrated by umh-core (config generation, process launching via S6)
- Handles all data flow processing (protocol converters, data flows, stream processors)
- Each instance runs as separate S6-supervised process

### Message Queue Architecture

**CRITICAL**: ManagementConsole does NOT have direct REST API access to umh-core. All communication flows through Redis-based message queues with a pull model.

#### Communication Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                     Management Console                          │
├──────────────────────┬──────────────────────────────────────────┤
│  Frontend (Svelte)   │  Backend (Go API)                        │
│  - User actions      │  - Validates requests                    │
│  - Display status    │  - Queues in Redis                       │
└──────────────────────┴──────────────────────────────────────────┘
                                    │
                          ┌─────────┴─────────┐
                          │   Redis Queues    │
                          │  (Message Broker) │
                          └─────────┬─────────┘
                                    │
┌───────────────────────────────────┴─────────────────────────────┐
│                         umh-core                                 │
├──────────────────────────────────────────────────────────────────┤
│  Communicator:                                                   │
│  - Puller: Polls /v2/instance/pull every 10ms                   │
│  - Pusher: Sends status via /v2/instance/push                   │
│                                                                  │
│  Agent:                                                          │
│  - Receives actions from Puller                                 │
│  - Routes to appropriate handlers                               │
│  - Triggers FSM transitions                                     │
│  - Updates config.yaml                                          │
│  - Generates benthos configs                                    │
│                                                                  │
│  FSM Controllers:                                                │
│  - BenthosFSM: Manages benthos-umh lifecycle                    │
│  - RedpandaFSM: Manages Kafka broker lifecycle                  │
│  - S6 FSM: Manages process supervision                          │
└──────────────────────────────────────────────────────────────────┘
```

#### API Endpoints (Management Console Backend)

**User-facing endpoints**:
- `POST /v2/user/push` - User sends actions to instances (via frontend)
- `GET /v2/user/pull` - User receives status updates (long polling)

**Instance-facing endpoints**:
- `POST /v2/instance/push` - Instance sends status to users (called by umh-core Pusher)
- `GET /v2/instance/pull` - Instance receives actions (polled by umh-core Puller every 10ms)

**Key architectural points**:
- **No direct instance → backend communication**: umh-core only knows about /v2/instance/* endpoints
- **Redis as broker**: Backend queues messages in Redis, instances poll to retrieve
- **10ms polling**: umh-core Puller checks for new actions 100 times per second
- **Bidirectional**: Actions flow backend → instance, status flows instance → backend

### Action Processing Flow

When a user deploys a bridge in the Management Console UI, here's the complete flow:

#### 1. Frontend Creates Action

```typescript
// Frontend sends action to backend
const action: ActionMessagePayload = {
  ActionType: "deploy-protocol-converter",
  ActionPayload: {
    id: "bridge-123",
    name: "PLC-Bridge",
    // ... bridge configuration
  },
  ActionUUID: crypto.randomUUID()
};

await fetch('/v2/user/push', {
  method: 'POST',
  body: JSON.stringify({
    Email: user.email,
    InstanceUUID: instance.id,
    Content: JSON.stringify(action)
  })
});
```

#### 2. Backend Validates and Queues

```go
// backend/cmd/v2/user_push.go
func (h *Handler) UserPush(w http.ResponseWriter, r *http.Request) {
    var msg models.UMHMessage
    json.NewDecoder(r.Body).Decode(&msg)

    // Validate message
    // Queue in Redis for instance to pull
    h.redis.QueueForInstance(msg.InstanceUUID, msg)
}
```

#### 3. umh-core Pulls Action

```go
// umh-core/pkg/communicator/api/v2/pull/pull.go
func (p *Puller) Start() {
    ticker := time.NewTicker(10 * time.Millisecond)
    for range ticker.C {
        messages := p.pullFromBackend() // GET /v2/instance/pull
        for _, msg := range messages {
            p.messageChannel <- msg
        }
    }
}
```

#### 4. Agent Routes Action

```go
// umh-core/pkg/agent/agent.go
func (a *Agent) processAction(msg models.UMHMessage) {
    var action models.ActionMessagePayload
    json.Unmarshal([]byte(msg.Content), &action)

    switch action.ActionType {
    case "deploy-protocol-converter":
        a.handleDeployProtocolConverter(action.ActionPayload)
    case "set-config-file":
        a.handleSetConfigFile(action.ActionPayload)
    // ... 50+ action types
    }
}
```

#### 5. FSM Processes Action

```go
// umh-core/pkg/fsm/benthos/reconcile.go
func (b *BenthosFSM) Reconcile(ctx context.Context) {
    // Read config.yaml (updated by agent)
    // Check current state vs desired state
    // Trigger appropriate actions:
    //   - Generate benthos config from template
    //   - Create S6 service directory
    //   - Launch benthos process via S6
}
```

#### 6. S6 Launches benthos-umh

```bash
# umh-core creates service directory
/data/services/benthos-bridge-123/
├── run                    # S6 run script
├── finish                 # S6 finish script
└── data/
    └── config.yaml        # Generated benthos config
```

#### 7. Status Flows Back

```go
// umh-core/pkg/communicator/api/v2/push/push.go
func (p *Pusher) Push(message models.UMHMessage) {
    // Check channel capacity
    if len(p.outboundMessageChannel) == cap(p.outboundMessageChannel) {
        p.logger.Warnf("Outbound message channel is full !")
        return
    }

    p.outboundMessageChannel <- message
    // Pusher sends to POST /v2/instance/push
}
```

### Action Types Reference

umh-core supports 50+ action types. Common ones include:

**Configuration Management**:
- `set-config-file` - Write complete config.yaml
- `get-config-file` - Read current config.yaml
- `patch-config-file` - Partial config updates

**Protocol Converters (Bridges)**:
- `deploy-protocol-converter` - Create/update bridge
- `edit-protocol-converter` - Modify existing bridge
- `delete-protocol-converter` - Remove bridge
- `start-protocol-converter` / `stop-protocol-converter` - Control bridge

**Data Flow Components**:
- `deploy-data-flow-component` - Create/update data flow
- `delete-data-flow-component` - Remove data flow
- `start-data-flow-component` / `stop-data-flow-component` - Control data flow

**Diagnostics**:
- `get-logs` - Retrieve service logs
- `get-status` - Request status update
- `restart-service` - Restart specific service

**Location**: See `umh-core/pkg/models/action_models.go` for complete list

### Template Variables and Expansion

**Key concept**: ManagementConsole NEVER writes benthos config directly. It only updates config.yaml in umh-core, which then generates benthos configs from templates.

#### Variable Sources

Template variables come from multiple sources:

1. **Connection config** (`protocolConverter.connection.variables`):
   ```yaml
   connection:
     name: "PLC-Connection"
     variables:
       IP: "192.168.1.100"
       PORT: 502
   ```

2. **Agent location** (`agent.location`):
   ```yaml
   agent:
     location:
       - enterprise: "ACME"
       - site: "Factory-1"
       - area: "Assembly"
   ```

3. **Bridge location** (`protocolConverter.location`):
   ```yaml
   protocolConverter:
     location:
       - line: "Line-A"
   ```

#### Variable Flattening

**IMPORTANT**: Nested variables become top-level in templates:

```yaml
# In config.yaml:
connection:
  variables:
    IP: "192.168.1.100"
    PORT: 502

# In benthos template:
{{ .IP }}    # NOT {{ .variables.IP }}
{{ .PORT }}  # NOT {{ .variables.PORT }}
```

#### Location Path Computation

Agent location + bridge location = `{{ .location_path }}`:

```yaml
# Agent location:
agent:
  location:
    - enterprise: "ACME"
    - site: "Factory-1"

# Bridge location:
protocolConverter:
  location:
    - line: "Line-A"
    - cell: "Cell-5"

# Results in:
# {{ .location_path }} = "ACME.Factory-1.Line-A.Cell-5"
```

#### Template Expansion Example

```yaml
# Template (in config.yaml):
templates:
  - id: "modbus-tcp"
    config: |
      input:
        label: "modbus_{{ .name }}"
        modbus_tcp:
          address: "{{ .IP }}:{{ .PORT }}"

      output:
        broker:
          outputs:
            - kafka:
                addresses: ["localhost:9092"]
                topic: "umh.v1.{{ .location_path }}.{{ .name }}"

# Bridge config:
protocolConverters:
  - id: "bridge-123"
    name: "PLC-Bridge"
    templateRef: "modbus-tcp"
    connection:
      name: "Factory-PLC"
      variables:
        IP: "192.168.1.100"
        PORT: 502
    location:
      - line: "Line-A"

# Generated benthos config (by umh-core Agent):
input:
  label: "modbus_PLC-Bridge"
  modbus_tcp:
    address: "192.168.1.100:502"

output:
  broker:
    outputs:
      - kafka:
          addresses: ["localhost:9092"]
          topic: "umh.v1.ACME.Factory-1.Line-A.PLC-Bridge"
```

### Cross-Repository Debugging Workflow

When debugging issues that span repositories, follow this systematic approach:

#### 1. Identify Symptom Location

**UI shows error** → Start in ManagementConsole frontend
**Status not updating** → Check ManagementConsole backend → umh-core Communicator
**Bridge stuck in "starting"** → umh-core FSM → benthos-umh process
**Data not flowing** → benthos-umh config → umh-core template expansion
**Process crash** → benthos-umh logs → S6 supervision

#### 2. Trace the Full Stack

For a bridge deployment issue:

```bash
# 1. Frontend: Check browser console
# Look for failed API calls, validation errors

# 2. Backend: Check if action was queued
# Look for /v2/user/push logs, Redis queue status

# 3. umh-core Communicator: Check if action was pulled
grep "Received action.*deploy-protocol-converter" /data/logs/umh-core/current

# 4. umh-core Agent: Check if action was processed
grep "Processing action.*bridge-123" /data/logs/umh-core/current

# 5. umh-core FSM: Check state transitions
grep "bridge-123.*FSM.*transition" /data/logs/umh-core/current

# 6. S6: Check if service was created
ls -la /data/services/*bridge-123*/

# 7. benthos-umh: Check process status and logs
tai64nlocal < /data/logs/benthos-bridge-123/current | tail -100

# 8. benthos config: Verify template expansion
cat /data/services/benthos-bridge-123/data/config.yaml
```

#### 3. Common Cross-Repository Issues

**Issue**: Bridge shows "starting" but benthos process is actually running and processing data

**Root Cause**: Communicator Pusher channel overflow prevents status updates from reaching backend

**Investigation**:
```bash
# Check umh-core logs for channel overflow
grep "Outbound message channel is full" /data/logs/umh-core/current

# Verify benthos is processing
rpk topic consume umh.v1.* --num 10  # Should show data flowing

# Check benthos logs
tai64nlocal < /data/logs/benthos-bridge-123/current | grep "INFO"
```

**Resolution**: This is a display bug, not a functional issue. Data is flowing correctly.

---

**Issue**: Template variables not expanding correctly

**Root Cause**: Variable flattening not understood, or location path computed incorrectly

**Investigation**:
```bash
# Check config.yaml structure
grep -A 20 "bridge-123" /data/config.yaml

# Check generated benthos config
cat /data/services/benthos-bridge-123/data/config.yaml

# Compare template vs generated
diff <(grep -A 50 "templates:" /data/config.yaml | grep -A 30 "modbus-tcp") \
     /data/services/benthos-bridge-123/data/config.yaml
```

**Resolution**: Fix variable references in template or ensure connection variables are defined.

---

**Issue**: Action sent from frontend but never received by umh-core

**Root Cause**: Network issues, instance offline, or Redis queue problems

**Investigation**:
```bash
# Check if action was queued in backend
# (requires backend logs or Redis access)

# Check umh-core Puller logs
grep "GET /v2/instance/pull" /data/logs/umh-core/current

# Check network connectivity
curl -v https://management.umh.app/v2/instance/pull
```

**Resolution**: Fix network connectivity, verify AUTH_TOKEN, check instance registration.

#### 4. Key Debugging Principles

1. **Actions flow one way**: Management Console → umh-core (never the reverse)
2. **Status flows the other way**: umh-core → Management Console
3. **Config.yaml is source of truth**: ManagementConsole updates config.yaml, umh-core reads it
4. **Templates are in config.yaml**: benthos-umh configs are GENERATED, not stored
5. **S6 is the process manager**: If benthos crashes, S6 restarts it
6. **FSM controls lifecycle**: State transitions trigger config generation and process management

#### 5. Repository-Specific Troubleshooting

**ManagementConsole Issues**:
- Check browser console for frontend errors
- Check backend logs for API validation failures
- Verify Redis connectivity if actions aren't queued

**umh-core Issues**:
- Check Communicator logs for pull/push failures
- Check Agent logs for action processing errors
- Check FSM logs for state transition failures
- Check S6 logs for process supervision issues

**benthos-umh Issues**:
- Check benthos process logs for config errors
- Check benthos metrics for processing throughput
- Verify input/output connectivity (Modbus, Kafka, etc.)

## Universal Troubleshooting Principles

This section provides high-level principles for effective troubleshooting in any UMH component.

### The Investigation Hierarchy

Follow this systematic approach to problem investigation:

1. **Verify the problem exists** - Check if UI matches reality (status vs metrics/throughput)
2. **Gather ALL context** - Comments, screenshots, related issues
3. **Look for patterns** - Similar issues, recent changes, cross-component problems
4. **Question assumptions** - "Starting" might mean "running but showing old status"
5. **Find the fix** - Often already exists in another PR/issue

### The Three Sources of Truth

Always check all three before concluding:

1. **What the UI shows** - May be cached/stale
2. **What the logs say** - May be from different time
3. **What's actually happening** - Metrics, network traffic, database queries

**Example**: Bridge shows "starting" in UI (source 1), logs show "running" state (source 2), Kafka topics show data flowing (source 3) → Display bug, not functional issue.

### Quick Wins Checklist

Before deep investigation, check these patterns:

- [ ] **Does restart fix it?** → State/cache/queue issue
- [ ] **Are metrics/throughput normal?** → Display issue only
- [ ] **Did it work before? When did it break?** → Check PRs in that window
- [ ] **Similar issues exist?** → Search Linear/GitHub broadly
- [ ] **Comments checked?** → Critical context lives there
- [ ] **Screenshots transcribed?** → Hidden evidence

### Investigation Smells

These patterns often indicate specific issue types:

**Status doesn't match performance** → Timestamp/cache bug
- Bridge shows "starting" but data is flowing
- UI shows error but service is healthy

**"Sometimes works"** → Race condition
- Config sync + manual edits
- Multiple browser tabs open
- Concurrent deployments

**"Used to work"** → Recent PR broke it
- Check merged PRs between "last worked" and "first failed"
- Look for related component changes
- Check dependency updates

**"Works after restart"** → State accumulation
- Message queue overflow
- Memory leak
- Stale cache

**Multiple customers, same issue** → Systematic problem
- Not edge case or misconfiguration
- Likely needs code fix, not workaround

**Gray/neutral status with throughput** → FSM display bug
- Data is actually flowing
- Status update not reaching UI
- Communicator channel overflow

### The "Why Does Restart Fix It?" Analysis

Understanding why restarts resolve issues reveals root causes:

**Restart fixes** → State/cache/queue issue
- Communicator message channel overflow
- FSM state corruption
- Cached config not refreshing
- **Solution**: Fix state management, increase buffer sizes, add monitoring

**Restart doesn't fix** → Configuration/network issue
- Wrong IP address or port
- Network routing problem
- Missing credentials
- **Solution**: Fix configuration, verify network connectivity

**Restart sometimes fixes** → Timing/race condition
- Multi-process config modification
- Startup order dependencies
- Network flakiness
- **Solution**: Add synchronization, fix race conditions, increase timeouts

**Restart temporarily fixes** → Resource leak/accumulation
- Memory leak in benthos process
- Goroutine leak in umh-core
- Growing message queue
- **Solution**: Fix resource leak, add resource limits, monitor growth

### Component Interaction Mindset

**Critical principle**: Issues often cross component boundaries. Never assume a symptom's location is the root cause's location.

**Common patterns**:
- FSM shows error → Actually benthos config validation failure
- Benthos won't start → Actually S6 supervision issue
- Protocol converter timeout → Actually network MTU fragmentation
- Data not flowing → Actually Kafka topic permission issue

**Always trace the full stack**:
```
User Action (Frontend)
  ↓
Backend Validation
  ↓
Redis Queue
  ↓
umh-core Communicator (Puller)
  ↓
umh-core Agent (Action Handler)
  ↓
umh-core FSM (State Machine)
  ↓
Config Generation (Template Expansion)
  ↓
S6 Supervision (Process Management)
  ↓
benthos-umh Process (Data Processing)
  ↓
Protocol/Data Source (External System)
```

**Debugging tip**: Start from the symptom, trace backwards to the root cause. Don't assume the symptom location is where the fix belongs.

## Common Development Tasks

**Adding a Data Flow Component**: Define in YAML → Add validation in `pkg/datamodel/` → Update Benthos FSM → Add tests

**Debugging FSM**: Enable `LOG_LEVEL=debug` → Check transitions in logs → Use `make test-debug`

**GraphQL changes**: Modify schema → `make generate` → Test with `make test-graphql`

**Bumping benthos-umh Version**:

*Step 1: Deep Research (understand EVERYTHING first)*

Before writing the PR, gather complete context:

1. **Identify version range**: Check current version in `umh-core/Makefile` line ~72, determine target version
2. **Read ALL benthos-umh releases** between versions: https://github.com/united-manufacturing-hub/benthos-umh/releases
3. **Read related Linear tickets**:
   - Main issue requesting the bump
   - Parent/child issues
   - Root cause issues referenced in release notes
   - **Read ALL comments** on each issue
4. **Read ALL related PRs**:
   - benthos-umh PRs that introduced the fixes
   - PR descriptions, code changes, review comments
   - ManagementConsole PRs that are blocked (if mentioned)
5. **Understand the user impact**:
   - What was broken from user perspective?
   - What symptoms did they see? (error messages, stuck states, etc.)
   - What manual workarounds existed?
   - How does the fix change their experience?

*Step 2: Condense for User Impact*

Transform technical commits into user-facing descriptions:
- ❌ "fix: use bytes.TrimRight() for S7 strings"
- ✅ "Fixed S7 bridges stuck in starting state due to NULL-padded strings"

Each bug fix should answer:
- What was broken? (observable symptoms)
- Why did it happen? (root cause in one sentence)
- What's fixed? (user experience improvement)
- Technical details? (inline, not separate section)

*Step 3: Create PR*

File to modify: `umh-core/Makefile` (line ~72: `BENTHOS_UMH_VERSION`)

PR format (matches UMH release notes):
```
This PR bumps benthos-umh from vX.Y.Z to vA.B.C

🐛 Bug Fixes

**[User-facing symptom]** (from vX.Y.Z+1)
[One concise paragraph: what was broken, why, what's fixed, technical details inline]

**[User-facing symptom]** (from vA.B.C)
[Same format]

📝 Notes

- [Why this bump is needed, urgency, blocking issues with links]
- Release Notes from benthos-umh: [vX.Y.Z+1](link) and [vA.B.C](link)
```

**Important**:
- Include ALL versions between current and target (jumping 0.11.3 → 0.11.5 needs both 0.11.4 and 0.11.5)
- Focus on user impact, not code changes
- Keep concise: bold title + inline version marker + one paragraph
- Add emoji sections (🐛, 💪, 📝) for visual hierarchy
- This PR description becomes the changelog for next umh-core release

**Example**: See PR #2284 for benthos-umh v0.11.3 → v0.11.5 bump

## Issue Investigation Workflow

When investigating FSM or service issues, follow this systematic approach:

### 1. Gather Context

**First, always ask the user for:**
- Action logs from UI/Management Console (if applicable)
- Description of what they were trying to do
- Timestamps and error messages

**Then check Linear context:**
- Read main issue and ALL sub-issues
- Review related PRs and their comments
- Understanding previous fix attempts is crucial

### 2. Analyze Logs Systematically

Start broad, then narrow:
```bash
# Recent errors and warnings (with human-readable timestamps)
tai64nlocal < /data/logs/umh-core/current | tail -1000 | grep -E "ERROR|WARN"

# FSM state changes for specific service
grep "service-name.*currentFSM" /data/logs/umh-core/current

# Find when issue started
grep -n "first-error-pattern" /data/logs/umh-core/* | head -1
```

**Key patterns to look for:**
- Empty FSMState (`FSMState=''`)
- Rapid retry loops (same timestamp repeating)
- State transitions that don't complete
- "Not existing" or "service does not exist" errors

### 3. Trace Code Paths

Map symptoms to source code:
1. Find where log messages originate: `grep -r "log message" pkg/`
2. Trace back through the FSM reconciliation loop
3. Identify blocking conditions or failed transitions

**FSM Structure (always the same):**
- `machine.go` - States and transitions
- `reconcile.go` - Control loop logic
- `actions.go` - Operations that can fail
- `models.go` - Data structures

### 4. Analyze Service State

```bash
# Build and run S6 analyzer for deep inspection
cd tools/s6-analyzer && go build && ./s6-analyzer /data/services/service-name

# Quick directory check
ls -la /data/services/*service-name*/
```

**Look for anomalies:**
- Down files blocking startup
- Missing supervise directories
- Timestamp mismatches
- Zombie services (directory exists but S6 returns empty)

### 5. Check Configuration

Verify the configuration chain:
- Template exists and is valid
- Variables are properly substituted
- References match actual resources

### 6. Build Timeline

Create a clear sequence:
1. User action → System response
2. FSM state transitions with timestamps (ISO-8601 with timezone, e.g., 2025-09-22T14:37:05Z)
3. Where it got stuck and why
4. Evidence from logs (quote exact lines)

### 7. Document Findings

Use the Linear template but focus on:
- **Root cause** (one clear sentence)
- **Evidence** (logs, config, S6 state)
- **Reproduction steps** (minimal and clear)
- **Why existing fixes didn't work** (if applicable)

## Code Path Analysis

When tracing issues through code:

### Understanding FSM Flow
Every FSM follows: **Event → State Change → Reconcile → Action → New State**

To trace issues:
1. Find the stuck state in reconcile.go
2. Check what condition prevents progress
3. Trace back to what updates that condition
4. Find why the update fails

### Common Patterns

**FSM Stuck**: Current state ≠ Desired state, reconciliation blocking
**Service Won't Start**: Check preconditions in reconcile, verify Create/Start idempotent
**Rollback Issues**: Timeout triggers rollback, cleanup assumes service exists

## Key Investigation Principles

1. **Start with user perspective** - What were they trying to do?
2. **Follow the data** - Logs don't lie, but they may be incomplete
3. **Verify assumptions** - Check if service actually exists before trying to stop it
4. **Consider timing** - Race conditions often appear as intermittent issues
5. **Check the full stack** - Protocol Converter → Benthos → S6 → Filesystem

## When to Dig Deeper

- Multiple customers report similar issues → Systematic problem
- Issue reoccurs after fix → Incomplete understanding
- Logs show impossible states → Race condition or corrupted state
- Rollback creates more problems → Non-idempotent operations

Remember: Every FSM issue has a trigger, a stuck state, and a missing transition. Find all three.

## UI Testing with Playwright MCP

When reproducing or testing UI-related issues, use Playwright MCP for browser automation:

### Setup and Navigation
```bash
# Start test environment
make test-no-copy  # Use current config without copying

# Navigate to Management Console
mcp__playwright__browser_navigate url: "https://management.umh.app"

# Take screenshots for documentation
mcp__playwright__browser_take_screenshot fullPage: true, filename: "before-deployment.png"
```

### Collaborative Workflow
The most effective approach combines human and AI capabilities:

1. **Human prepares context**: User creates initial setup, navigates to relevant page
2. **AI traces actions**: Uses browser_snapshot to understand current state
3. **Human provides credentials**: Login, sensitive data entry
4. **AI performs repetitive tasks**: Clicking through deployment flows, waiting for timeouts
5. **Both observe results**: Human confirms visual state, AI analyzes logs

### Key Capabilities
- **State observation**: `browser_snapshot` provides accessibility tree for navigation
- **Action automation**: Click buttons, fill forms, wait for conditions
- **Evidence collection**: Screenshots (though not automatically saved to PR)
- **Multi-tab handling**: Track deployment dialogs and logs simultaneously

### Testing Protocol Converter Deployments
```yaml
# Example reproduction workflow:
1. Navigate to Data Flows page
2. Click on protocol converter to edit
3. Change protocol type (e.g., S7 → Generate)
4. Click "Save & Deploy"
5. Monitor deployment dialog for status changes
6. Wait for timeout/success
7. Check logs for FSM state transitions
8. Screenshot final state for documentation
```

### Best Practices
- **Always screenshot before/after**: Provides visual evidence for reports
- **Monitor both UI and logs**: Deployment dialog + backend FSM states
- **Document timing**: Note when "not existing" states appear
- **Capture error messages**: Exact text from UI alerts and dialogs
- **Test multiple scenarios**: Failed deployments, successful deployments, rollbacks

### Limitations and Improvements
- Screenshots aren't automatically attached to PRs (manual step needed)
- Browser console errors should be checked with `browser_console_messages`
- Network requests can be monitored with `browser_network_requests`
- For complex forms, use `browser_fill_form` for batch field updates


## Important Notes

- **Default branch for PRs**: `staging` (not main)
- **Focused tests**: Do not commit focused specs. CI runs with `--fail-on-focused` and will fail if any are present
- **FSM callbacks**: Keep them fail-free (logging only)
- **Actions**: Must be idempotent and handle context cancellation
- **Exponential backoff**: System automatically retries failed transitions
- **Resource Limiting**: Bridge creation is blocked when resources are constrained (controlled by `agent.enableResourceLimitBlocking` feature flag)

## UX Standards

See `UX_STANDARDS.md` for UI/UX principles when building management interfaces or user-facing components.
