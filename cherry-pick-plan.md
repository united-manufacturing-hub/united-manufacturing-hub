# Cherry-Pick Plan for topic-browser-opti Branch - REVISED

## PR #1: Parallel Manager & Time Budget Control
**Branch: `pr1-parallel-manager`**

### Commits to cherry-pick (in chronological order):
```bash
8b39e553b  # feat: prep mutex
4269d2fa3  # feat: parallel manager  
22678b89a  # feat: also use inner for loop
be10eda42  # save (interim save during parallel work)
fe2f28df9  # feat: 80% ctx for inner reconcile (CONTROL LOOP)
421c9692b  # fix: math (CONTROL LOOP)
da57dcc3e  # 90% (CONTROL LOOP)
e9b3672fb  # 95% (CONTROL LOOP)
24116c4de  # feat: 80% ctx for inner reconcile (BASE MANAGER)
b2a7215a5  # fix: math (BASE MANAGER)  
3d81c8151  # 90% (BASE MANAGER)
201eb77ee  # 95% (BASE MANAGER)
52a195872  # fix: use correct ctx
623464060  # fix: return ErrNoDeadline (BASE MANAGER)
97b8143ec  # fix: ErrNoDeadline (CONTROL LOOP)
35672280a  # fix: Use constant (extract hardcoded factors)
c2135a187  # fix: sync
```

## PR #2: S6 Service Lifecycle & Logging Fixes
**Branch: `pr2-s6-lifecycle`**
**Depends on: pr1-parallel-manager**

### Commits to cherry-pick (in order):
```bash
109a0d224  # fix: prevent s6 race condition where service output bypasses logging
ba73f5651  # feat(s6): Fix S6 service lifecycle management and log service startup
15bd1cd7b  # Fix multiple s6-supervise processes issue
298e2b912  # Fix S6 binary blob pollution in Docker logs
e8b293696  # fix: optimize buffer allocation based on S6 line limit analysis
bf7285eff  # Implement simplified S6 service logging plan
318253e73  # fix: improve S6 rotation handling and test coverage
```

## PR #3: FSM Restart Resilience & Error Handling
**Branch: `pr3-fsm-resilience`**
**Depends on: pr1-parallel-manager**

### Commits to cherry-pick (in order):
```bash
a5872ef0a  # fix: resolve FSM restart deadlock with resilient error handling
```

## PR #4: FSM Logging, Debugging & Timing Calibration
**Branch: `pr4-fsm-logging`**
**Depends on: pr2-s6-lifecycle, pr3-fsm-resilience**

### Commits to cherry-pick (in order):
```bash
4da1072c6  # perf: optimize FSM timing constants
01656da53  # debug: enhance FSM debugging and logging
8edc25d1b  # Revert "debug: enhance FSM debugging and logging"
53b701bec  # refactor: replace verbose debug logs with useful status info
ca593cfcb  # Complete observed state debug cleanup for remaining FSMs
3be6f3a41  # Reduce LoopControlLoopTimeFactor to 0.80 for better cancellation buffer
```

## PR #5: Topic Browser Memory & Parsing Optimizations
**Branch: `pr5-topic-browser`**
**Depends on: pr4-fsm-logging**

### Commits to cherry-pick (in order):
```bash
ab7df54a5  # feat: optimize Topic Browser memory usage - 73% reduction achieved
fb25b77bb  # feat: Phase 4 - Buffer pool optimization for Topic Browser hex parsing
c51dfa423  # feat: add resilient error handling to skip malformed blocks
af8ad8d54  # reduced memory management even more
bc3e007cf  # Implement block processing prevention and enhance buffer management in Topic Browser
7eda35af9  # perf: optimize Topic Browser string searching for 48x performance improvement
c780d3804  # fix(topicbrowser): implement Priority 1 fixes and consolidate documentation
```

## Commits to Skip (TRUE WIP/Debug/Reverted):
- `73cc85953` - dbg logs (just debug logging)
- `229c98e51` - test: no encode (debug test)
- `b5b6a13e6` - save (empty WIP commit)
- `e87b4b98c` - fix: printf (debug print fix)
- `a6fc27694` - save (empty WIP commit)
- `93aef1ccc` - topic-browser-opti (branch marker)
- `841b14d19` - fix: prevent ServiceExists race condition after umh-core restart (reverted)
- `a5ddcfbe9` - Revert "fix: prevent ServiceExists race condition after umh-core restart"
- `2acc7d9ce` - [ENG-3079] reject child PC editing via UI (#2118) (already merged)
- `64d401b1f` - [ENG-1242] Pause DFC (#2053) (already merged)

## Key Insights:
1. **Two parallel time budget implementations**: Control loop and base manager each got their own evolution from 80% → 90% → 95%
2. **Critical infrastructure commits**: Error handling, context fixes, and constants were essential for the parallel system
3. **Proper sequencing**: The commits build on each other and need to be applied in chronological order

## Updated Cherry-Pick Commands

### Step 1: Create PR #1 branch
```bash
git checkout staging
git checkout -b pr1-parallel-manager
git cherry-pick 8b39e553b 4269d2fa3 22678b89a be10eda42 fe2f28df9 421c9692b da57dcc3e e9b3672fb 24116c4de b2a7215a5 3d81c8151 201eb77ee 52a195872 623464060 97b8143ec 35672280a c2135a187
```

### Step 2: Create PR #2 branch
```bash
git checkout pr1-parallel-manager
git checkout -b pr2-s6-lifecycle
git cherry-pick 109a0d224 ba73f5651 15bd1cd7b 298e2b912 e8b293696 bf7285eff 318253e73
```

### Step 3: Create PR #3 branch
```bash
git checkout pr1-parallel-manager
git checkout -b pr3-fsm-resilience
git cherry-pick a5872ef0a
```

### Step 4: Create PR #4 branch
```bash
git checkout pr2-s6-lifecycle
git checkout -b pr4-fsm-logging
git cherry-pick pr3-fsm-resilience  # merge pr3 first
git cherry-pick 4da1072c6 01656da53 8edc25d1b 53b701bec ca593cfcb 3be6f3a41
```

### Step 5: Create PR #5 branch
```bash
git checkout pr4-fsm-logging
git checkout -b pr5-topic-browser
git cherry-pick ab7df54a5 fb25b77bb c51dfa423 af8ad8d54 bc3e007cf 7eda35af9 c780d3804
``` 