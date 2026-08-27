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
./run.sh                 # no quota, runs until Ctrl+C
CPUS=2 ./run.sh          # two CPUs of quota
DURATION=60s ./run.sh    # stop on its own instead
LOG_LEVEL=info ./run.sh  # only state changes, not every poll
```

It runs until you stop it, so you can load the machine and watch the readings
answer while it keeps going.

The runner's log carries the supervisor's own lines as well as the monitor's.
To watch just the readings:

```bash
./run.sh --name cpu-host | grep --line-buffered cpu_reading
```

`--line-buffered` is not optional on an endless run. Without it grep holds
output in a block buffer and prints nothing until the buffer fills, so a
working run looks like a dead one.

⚠️ Keep the pipe out of a first run. A machine that never takes a reading has
nothing matching `cpu_reading` to print, so the filter turns a startup error
into silence, and an empty result cannot tell you which of the two happened.
Run it bare once, confirm readings appear, then add the pipe.

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
  Run with a quota and a name, then load it from a second terminal. The
  container has `stress-ng` installed, so that terminal needs nothing of its
  own:

  ```bash
  CPUS=2 ./run.sh --name cpu-host
  docker exec cpu-host stress-ng --cpu 8 --timeout 10
  ```

  Eight workers against two CPUs is a deliberate 4x overcommit, so
  `nr_throttled` climbs at once rather than after a wait. Ten seconds of load is
  enough: the throttling ratio is a delta over a 60-second window, so a short
  burst fires it, and stopping early leaves you watching the half that is easy to
  miss.

  ⚠️ **The verdict will not go back to healthy, and that is a defect rather than
  something you are doing wrong.** Throttling decays to 0% about a minute after
  the load stops, which is the 60-second window draining and is correct. The
  verdict then stays `degraded` anyway: a latched signal's release is gated on a
  window-coverage check that a jittered clock never satisfies, so nothing clears
  it short of a restart. Measured 2026-08-27; the fix is not in this branch.

  Until that lands, read `Throttling 0%` beside a `degraded` verdict as the known
  bug, not as evidence about your machine.

  Do not use `CPUS=0.5`. It is under umh-core's own minimum, so the reserves the
  monitor subtracts leave numbers that describe no machine we support.

- **Load on the host** competes for the machine's own CPUs and moves
  host-busy and steal as read from `/proc/stat`, leaving throttling
  untouched. Run the scenario without `CPUS` for this world: an unquota'd
  machine can only be filled by its host.

A `--timeout` ends the load by itself. Without one, stop it with
`docker kill cpu-host`: `Ctrl+C` in the second terminal stops only the
`docker exec` client, and the load keeps running inside the container.

Stop the monitor itself with `Ctrl+C` in the first terminal.
