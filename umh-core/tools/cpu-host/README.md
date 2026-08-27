# cpu-host

Runs umh-core's CPU health monitor against a real Linux machine and prints
its readings, so you can watch how it judges a machine under load. The other
CPU example scenarios run the monitor against a fake machine whose numbers
are chosen to trip a specific signal. This one stages nothing: the monitor
reads whatever CPU accounting files the machine actually publishes, and its
verdicts follow from those numbers alone.

## Why a container

The monitor reads the cgroup v2 file `/sys/fs/cgroup/cpu.stat` and the host
file `/proc/stat`. Linux publishes both; macOS and Windows publish neither.
Run the scenario directly on such a machine and it stops immediately with
this error, because no reading can ever land there:

```
this host publishes no cgroup v2 CPU files, so no reading can land: stat
/sys/fs/cgroup/cpu.stat: no such file or directory; run it under
tools/cpu-host, which puts it in a Linux container
```

A container provides the files, and with a CPU quota it provides one more
thing a bare Linux host usually lacks: a real limit, so the throttling
signal has something to observe. The monitor reads only CPU files, so
limiting memory changes nothing it can see.

## Prerequisites

- Docker
- A Go toolchain
- This repository checked out. The script builds the scenario runner from
  `pkg/fsmv2/cmd/runner`

## Running

```bash
./run.sh                 # no quota, 60 seconds
CPUS=0.5 ./run.sh        # half a CPU of quota
DURATION=10m ./run.sh    # watch longer
LOG_LEVEL=info ./run.sh  # quieter; per-poll readings need debug
```

Any argument is passed to `docker run` verbatim, so docker's own options
work without the script naming them. `--name` is the useful one: it makes
the container addressable for the load experiment below.

## What to watch

The monitor runs as one worker inside the scenario runner, and its readings
show up in that runner's log. On every completed poll the `cpu_reading` debug
line carries the verdict and the composed message; a failed poll never
reaches it. When the worker's state changes, the message also appears at info
in the `state_transition` line's `reason` field.

Two kinds of load move different signals, and telling them apart is the point
of the exercise:

- **Load inside the container** competes for the container's quota and
  moves the throttling signal (`nr_throttled`) and cgroup pressure (PSI).
  Run with a quota and a name, then spin from a second terminal:

  ```bash
  CPUS=0.5 ./run.sh --name cpu-host
  docker exec cpu-host sh -c 'while :; do :; done'
  ```

- **Load on the host** competes for the machine's own CPUs and moves
  host-busy and steal as read from `/proc/stat`, leaving throttling
  untouched. Run the scenario without `CPUS` for this world: an unquota'd
  machine can only be filled by its host.

Stop the in-container load with `docker kill cpu-host`. `Ctrl+C` in the
second terminal does not do it: the interrupt stops only the `docker exec`
client, and the load keeps running inside the container.
