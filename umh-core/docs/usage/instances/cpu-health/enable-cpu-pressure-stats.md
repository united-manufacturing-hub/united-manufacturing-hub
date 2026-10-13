# Enable CPU pressure stats

{% hint style="info" %}
**Early access.** UMH Core reads CPU pressure only with CPU Health turned on: v0.44.37 or later,
started with `-e USE_FSMV2_CPU=true`. See [CPU Health](README.md).
{% endhint %}

Follow this page when the Technical Details `Pressure` line reads `Pressure not measured (this
operating system does not report CPU pressure stats).`, for example on an instance with the status
**CPU healthy, limited visibility**. Until then, CPU health still reports from throttling, steal and
headroom. When this page works, the `Pressure` line shows a percentage.

UMH Core reads CPU pressure from the container's own `/sys/fs/cgroup/cpu.pressure`. That file exists
only if the kernel booted with Pressure Stall Information (PSI) on and the machine runs cgroup v2.
Most distributions ship both on. The Red Hat family is the exception: RHEL, Rocky, AlmaLinux and
Oracle Linux ship PSI off, and version 8 of each also boots cgroup v1.

Check your own machine:

```bash
docker exec <container name> cat /sys/fs/cgroup/cpu.pressure
```

A line beginning `some avg10=` means there is nothing to do. `No such file or directory` means PSI,
cgroup v2, or both are missing. To find out which, run this on the machine, outside the container:

```bash
stat -fc %T /sys/fs/cgroup
```

`cgroup2fs` means only PSI is missing. Set `psi=1` as described below. `tmpfs` means the machine
runs cgroup v1, which has no `cpu.pressure` file even with `psi=1`. On cgroup v1, set
a CPU limit on the container instead, so that UMH Core measures headroom against the limit. UMH Core
still reads the container's usage, throttling and CPU limit from the v1 controller files, and
machine headroom and steal from `/proc/stat`. Pressure is the only signal it cannot read there.

## Set the kernel parameters

{% hint style="warning" %}
The kernel parameter takes effect after a reboot. Rebooting the machine stops UMH Core and every
bridge on it until the machine is back up. Plan the reboot for a maintenance window.
{% endhint %}

Each operating system sets kernel parameters differently. Follow yours, then reboot:

| Operating system | Instructions |
|------------------|--------------|
| RHEL, Rocky, AlmaLinux, Oracle Linux, Fedora | [Configuring kernel command-line parameters](https://docs.redhat.com/en/documentation/red_hat_enterprise_linux/9/html/managing_monitoring_and_updating_the_kernel/configuring-kernel-command-line-parameters_managing-monitoring-and-updating-the-kernel): `sudo grubby --update-kernel=ALL --args="psi=1"` |
| Debian and Ubuntu | [GRUB 2 setup](https://help.ubuntu.com/community/Grub2/Setup): add `psi=1` to `GRUB_CMDLINE_LINUX` in `/etc/default/grub`, then run `sudo update-grub` |

`update-grub` is a Debian and Ubuntu wrapper. On another GRUB machine, edit `GRUB_CMDLINE_LINUX` the
same way and run `grub-mkconfig -o /boot/grub/grub.cfg`.

## Confirm UMH Core can read cpu.pressure

Run the `docker exec` check again. It reads the same file UMH Core reads.

The Management Console shows the result on the instance's detail page. The Technical Details
`Pressure` line changes from `Pressure not measured (this operating system does not report CPU
pressure stats).` to a percentage. `Pressure not available (measuring).` means UMH Core can read the
file and is filling its 60-second window. Wait a minute and look again.

If the file is still missing, run `cat /proc/pressure/cpu` on the machine. Output means PSI is on and
cgroup v2 is what is missing. `No such file or directory` means the kernel ignored `psi=1`, which
happens on a kernel built without PSI. In both cases, set a CPU limit on the container instead. UMH
Core then measures headroom against the limit, and the `Pressure` line keeps reading `not measured`.
