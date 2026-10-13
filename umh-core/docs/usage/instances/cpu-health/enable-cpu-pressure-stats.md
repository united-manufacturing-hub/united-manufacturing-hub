# Enable CPU pressure stats

UMH reads CPU pressure from the container's own `/sys/fs/cgroup/cpu.pressure`. That file exists only
if the kernel booted with Pressure Stall Information (PSI) on and the machine runs cgroup v2. Most
distributions ship both on. The Red Hat family is the exception: RHEL, Rocky, AlmaLinux and Oracle
Linux ship PSI off, and version 8 of each also boots cgroup v1.

Check your own machine:

```bash
docker exec umh-core cat /sys/fs/cgroup/cpu.pressure
```

A line beginning `some avg10=` means there is nothing to do. `No such file or directory` means one or
both are missing. Find out which, on the host:

```bash
stat -fc %T /sys/fs/cgroup
```

`cgroup2fs` means only PSI is missing: set `psi=1`. `tmpfs` means the machine runs cgroup v1, where
`cpu.pressure` does not exist at all.

On cgroup v1, UMH reads the container's usage, throttling and CPU limit from the v1 controller files,
and machine headroom and steal from `/proc/stat`. CPU pressure is the only signal it cannot read, because
cgroup v1 has no per-container pressure file.

## Set the kernel parameters

Each operating system sets kernel parameters differently. Follow yours, then reboot:

| Operating system | Instructions |
|------------------|--------------|
| RHEL, Rocky, AlmaLinux, Oracle Linux, Fedora | [Configuring kernel command-line parameters](https://docs.redhat.com/en/documentation/red_hat_enterprise_linux/9/html/managing_monitoring_and_updating_the_kernel/configuring-kernel-command-line-parameters_managing-monitoring-and-updating-the-kernel): `sudo grubby --update-kernel=ALL --args="psi=1"` |
| Debian and Ubuntu | [GRUB 2 setup](https://help.ubuntu.com/community/Grub2/Setup): add `psi=1` to `GRUB_CMDLINE_LINUX` in `/etc/default/grub`, then run `sudo update-grub` |

`update-grub` is a Debian and Ubuntu wrapper. On another GRUB machine, edit `GRUB_CMDLINE_LINUX` the
same way and run `grub-mkconfig -o /boot/grub/grub.cfg`.

## Confirm UMH can read the file

Run the `docker exec` check again. It reads the same file UMH reads.

The Management Console shows the result on the instance's detail page. The Technical Details
`Pressure` line changes from `Pressure not available (not possible).` to a percentage. `Pressure not
available (measuring).` means UMH can read the file and is filling its 60-second window, so wait a
minute and look again.

If the file is still missing, run `cat /proc/pressure/cpu` on the host. Output means PSI is on and
cgroup v2 is what is missing. `No such file or directory` means the kernel ignored `psi=1`, which
happens on a kernel built without PSI. If the kernel has no PSI, set a CPU limit on the container
instead. UMH then judges the container against its limit. Pressure stays unavailable.
