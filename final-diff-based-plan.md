# Clean PR Plan Based on Final State

## PR #1: Core Parallel Manager & Time Budget System
**Branch: `pr1-parallel-manager-clean`**

### Changes:
- `pkg/control/loop.go`: Add parallel manager execution with errgroup
- `pkg/fsm/base_manager.go`: Add time-budget allocation for child FSMs  
- `pkg/constants/base_manager.go`: New file with `BaseManagerControlLoopTimeFactor = 0.95`
- `pkg/constants/loop.go`: Add `LoopControlLoopTimeFactor = 0.80`

### Key Features:
- Parallel execution of FSM managers using errgroup
- Time budget allocation (80% for control loop, 95% for base managers)
- Proper mutex protection for manager times
- Context deadline management with `ctxutil.ErrNoDeadline`

## PR #2: FSM Performance & Timing Optimizations  
**Branch: `pr2-fsm-performance-clean`**

### Changes:
- `pkg/constants/baseFSM.go`: Reduce cooldown timers (15→3-10 ticks)
- `pkg/constants/benthos.go`: Optimize execution times (40→35ms)
- `pkg/constants/s6.go`: Optimize execution times (30→25ms)  
- `pkg/constants/redpanda.go`: Optimize execution times
- `pkg/constants/dataflowcomponent.go`: Optimize execution times (45→40ms)
- `pkg/constants/topicbrowser.go`: Optimize execution times

### Key Features:
- Faster system responsiveness with reduced cooldowns
- Optimized P95 execution time expectations
- Better time budget allocation across all FSM types

## PR #3: S6 Service Lifecycle & Buffer Management
**Branch: `pr3-s6-improvements-clean`**

### Changes:
- `pkg/service/s6/`: Various lifecycle and logging improvements
- `pkg/fsm/s6/`: S6 FSM optimizations
- `pkg/constants/s6.go`: Add file read buffer constants

### Key Features:
- Improved S6 service lifecycle management
- Better log rotation and buffer handling
- File read optimizations with proper chunking

## PR #4: Topic Browser Memory & Performance Optimizations
**Branch: `pr4-topic-browser-clean`**

### Changes:
- `pkg/service/topicbrowser/`: Memory and parsing optimizations
- `pkg/communicator/topicbrowser/`: Cache and performance improvements
- `pkg/fsm/topicbrowser/`: FSM optimizations
- `pkg/constants/loop.go`: Add `RingBufferCapacity = 3` (62% memory reduction)

### Key Features:  
- 73% memory usage reduction for Topic Browser
- Buffer pool optimization for hex parsing
- Resilient error handling for malformed blocks
- 48x performance improvement in string searching

## PR #5: Infrastructure & Dependencies
**Branch: `pr5-infrastructure-clean`**

### Changes:
- `go.mod` / `go.sum`: Updated dependencies
- `vendor/`: LZ4 compression library updates
- `.gitignore`: Additional ignores
- `Dockerfile` / `Makefile`: Build improvements

### Key Features:
- Updated compression libraries for better performance
- Build system improvements

## Benefits of This Approach:

1. **Clean separation** of concerns - each PR focuses on one logical area
2. **No merge conflicts** from WIP commits and iterations  
3. **Testable increments** - each PR can be tested independently
4. **Clear dependencies** - PR #2 builds on PR #1, etc.
5. **Final optimized state** - we get the tested, working version immediately

## Execution Plan:

1. Create each branch from staging
2. Apply the final diff for that PR's files using `git apply`
3. Test each PR independently
4. Merge in order: PR #1 → PR #2 → PR #3 → PR #4 → PR #5 