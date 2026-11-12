# Enable CPU pressure stats

{% hint style="info" %}
UMH Core reads CPU pressure only while the CPU health check is on. It is on by default from
v0.44.46. See [CPU Health](README.md).
{% endhint %}

Follow this page when the Technical Details `Pressure` line reads `Pressure not measured (this
operating system does not report CPU pressure stats).`, for example on an instance with the status
**CPU healthy, limited visibility**. Without CPU pressure stats and without a CPU limit, UMH Core
judges CPU health on machine headroom, plus steal on a virtual machine. Once the kernel reports
pressure to the container, the `Pressure` line in Technical Details shows a percentage.

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

- `cgroup2fs` means only PSI is missing. Set `psi=1` as described below.
- `tmpfs` means the machine runs cgroup v1, which has no `cpu.pressure` file even with `psi=1`. Set
  a CPU limit on the container instead. With a limit, UMH Core measures throttling and the headroom
  left under the limit, so it can still tell when the instance runs short of CPU. On cgroup v1, UMH
  Core still reads the container's usage, throttling and CPU limit from the v1 controller files, and
  the machine's headroom and steal from `/proc/stat`. Pressure is the only signal it cannot read
  there.

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

If the file is still missing, run these checks on the machine, outside the container:

1. Run `cat /proc/cmdline`. If the output does not contain `psi=1`, the kernel did not get the
   parameter. Repeat the step for your operating system, then reboot.
2. If `psi=1` is there, run `cat /proc/pressure/cpu`. `No such file or directory` means the kernel
   was built without PSI and ignores `psi=1`. Set a CPU limit on the container instead.
3. If `/proc/pressure/cpu` prints output, the machine reports pressure but the container does not
   see it. Run `docker exec <container name> stat -fc %T /sys/fs/cgroup`. Any output other than
   `cgroup2fs` means the container runs without cgroup v2. Set a CPU limit on the container instead.
4. If the container prints `cgroup2fs`, the machine and the container both support pressure, and
   UMH Core should read it. Set a CPU limit on the container for now, and send the output of the
   three checks above to us on [Discord](https://discord.gg/F9mqkZnm8U) or through your usual UMH
   contact.

With a CPU limit, UMH Core measures throttling and the headroom left under the limit, and the `Pressure`
line keeps reading `not measured`.
