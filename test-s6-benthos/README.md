# S6 Benthos Integration Test

This test demonstrates how to spawn multiple Benthos instances via s6 service management. It creates a containerized environment where a Go test program manages the lifecycle of several Benthos services through s6.

## Overview

The test creates 5 Benthos instances, each running as an s6-managed service with:
- HTTP server input on ports 8080-8084
- Stdout output for message processing
- Metrics endpoints on ports 9080-9084
- Individual s6 service management (start/stop/restart)
- Logging via s6-log

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Docker Container                         │
│  ┌─────────────────────────────────────────────────────────┤
│  │                    s6-overlay                           │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────────┐ │
│  │  │   s6-svscan │  │  s6-rc      │  │  Go Test Prog   │ │
│  │  └─────────────┘  └─────────────┘  └─────────────────┘ │
│  │           │               │                 │           │
│  │           └───────────────┼─────────────────┼───────────┤
│  │                           │                 │           │
│  │  ┌─────────────────────────┼─────────────────┼─────────┐ │
│  │  │                        │                 │         │ │
│  │  │  ┌─────────────┐  ┌────▼────┐  ┌────────▼────┐   │ │
│  │  │  │ benthos-0   │  │benthos-1│  │ benthos-2   │   │ │
│  │  │  │ :8080       │  │ :8081   │  │ :8082       │   │ │
│  │  │  └─────────────┘  └─────────┘  └─────────────┘   │ │
│  │  │                                                  │ │
│  │  │  ┌─────────────┐  ┌─────────────┐               │ │
│  │  │  │ benthos-3   │  │ benthos-4   │               │ │
│  │  │  │ :8083       │  │ :8084       │               │ │
│  │  │  └─────────────┘  └─────────────┘               │ │
│  │  └──────────────────────────────────────────────────┘ │
│  └─────────────────────────────────────────────────────────┤
└─────────────────────────────────────────────────────────────┘
```

## Directory Structure

```
test-s6-benthos/
├── Dockerfile              # Container definition with s6 + benthos
├── docker-compose.yml      # Docker Compose configuration
├── Makefile                # Build and run commands
├── go.mod                  # Go module definition
├── cmd/
│   └── main.go             # Main test program
├── internal/
│   ├── s6manager/
│   │   └── manager.go      # S6 service management logic
│   └── testservice/
│       ├── filesystem.go   # Filesystem operations
│       └── s6.go          # S6 service interface
└── README.md               # This file
```

## Usage

### Quick Start

```bash
# Build and run the test
make test

# Or using docker-compose
docker-compose up --build
```

### Manual Steps

```bash
# Build the Docker image
make build

# Run the test
make run

# Run with verbose s6 logging
make run-verbose

# Run in background
make run-background

# View logs
make logs

# Open shell for debugging
make shell

# Clean up
make clean
```

### Testing Individual Services

Once running, you can test the Benthos instances:

```bash
# Test benthos-0
curl -X POST http://localhost:8080/ -d '{"test": "message"}'

# Test benthos-1  
curl -X POST http://localhost:8081/ -d '{"test": "message"}'

# Check metrics (benthos-0)
curl http://localhost:9080/metrics

# Check metrics (benthos-1)
curl http://localhost:9081/metrics
```

## How It Works

1. **Container Initialization**: s6-overlay starts as PID 1
2. **Go Program Launch**: The test program starts as an s6 service
3. **Service Creation**: For each Benthos instance:
   - Creates s6 service directory structure in `/data/services/`
   - Generates Benthos YAML configuration
   - Creates s6 run scripts (main + log)
   - Creates symlinks in `/run/service/` for s6-svscan
4. **Service Management**: 
   - Uses s6-svc commands to start/stop services
   - Monitors service status via s6-svstat
   - Handles service failures and restarts
5. **Logging**: Each service logs to `/data/logs/<service-name>/`

## Key Features Demonstrated

- **Multiple Service Management**: 5 concurrent Benthos instances
- **s6 Integration**: Full s6 service lifecycle (create/start/stop/remove)
- **Service Monitoring**: Status checking and automatic restart
- **Resource Management**: Memory limits and log rotation
- **Graceful Shutdown**: Proper cleanup on SIGTERM/SIGINT
- **Configuration Management**: Dynamic Benthos config generation
- **Port Management**: Automatic port allocation
- **Log Management**: s6-log with rotation and filtering

## Testing Scenarios

The test validates:

1. **Service Creation**: All services start successfully
2. **HTTP Endpoints**: Each Benthos instance serves HTTP requests
3. **Service Monitoring**: Status reporting works correctly
4. **Failure Recovery**: Services restart after failures
5. **Resource Isolation**: Services run independently
6. **Graceful Shutdown**: Clean termination handling
7. **Log Management**: Proper log file creation and rotation

## Configuration

Environment variables:
- `S6_VERBOSITY`: s6 logging level (0-3, default: 2)
- `S6_CMD_WAIT_FOR_SERVICES_MAXTIME`: Service startup timeout

Customizable parameters in `cmd/main.go`:
- `numBenthosInstances`: Number of Benthos services (default: 5)
- `testDuration`: How long to run the test (default: 2 minutes)

## Troubleshooting

### Check s6 Status
```bash
# Inside container
s6-svstat /run/service/*
```

### View Service Logs
```bash
# Inside container  
tail -f /data/logs/benthos-test-0/current
```

### Debug Service Issues
```bash
# Run with shell access
make shell

# Check s6 processes
ps aux | grep s6

# Check service directories
ls -la /run/service/
ls -la /data/services/
```

### Common Issues

1. **Port Conflicts**: Ensure ports 8080-8084 and 9080-9084 are available
2. **Memory Issues**: Increase Docker memory if services fail to start
3. **Permission Issues**: Check that s6 can create files in mounted volumes
4. **Service Startup**: Allow time for s6-svscan to detect new services

## Integration with UMH Core

This test demonstrates patterns used in the UMH Core codebase:

- Similar s6 service management as in `umh-core/pkg/service/s6/`
- Benthos configuration generation like `umh-core/pkg/service/benthos/`
- Filesystem abstraction matching `umh-core/pkg/service/filesystem/`
- FSM patterns compatible with `umh-core/pkg/fsm/`

The test serves as a validation environment for s6 service management improvements and can be extended to test additional scenarios.
