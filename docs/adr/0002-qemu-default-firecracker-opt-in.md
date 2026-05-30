# ADR 0002 — QEMU is the default monitor; Firecracker is opt-in and verified

**Status:** Accepted · **Date:** 2026-05

## Context
urunc supports several VM/sandbox monitors with very different boot times and
trusted-computing-base sizes (see [docs/URUNC.md](../URUNC.md) §3). Our practical
tools use **Linux** images, which per the urunc support matrix can only boot on
**QEMU or Firecracker** — Solo5 is Rumprun/MirageOS only. The urunc docs also
note that Linux-on-Firecracker is version-sensitive and can fail to boot.

## Decision
- Default every Linux tool to **QEMU** (broadest compatibility, most tested).
- Allow a tool to opt into **Firecracker** via `monitor: firecracker` in policy,
  applied as `--annotation com.urunc.unikernel.hypervisor=firecracker`.
- Gate that opt-in behind `make verify-monitors`, which honestly probes whether
  each monitor can boot a Linux image *in this host* before we rely on it. If
  Firecracker can't boot here, the tool stays on QEMU and we say so.
- Never override the baked hypervisor annotation on prebuilt **unikernel** images
  (e.g. `nginx_showcase`): their publisher set it correctly.

## Consequences
- Reliability by default; speed where the host proves it works.
- The monitor-diversity story is told truthfully (QEMU workhorse + a real
  Unikraft unikernel showcase) instead of claiming Solo5 runs our Linux tools.

## References
- Support matrix & hypervisor trade-offs: <https://urunc.io/hypervisor-support/>
- Annotation override mechanism: <https://urunc.io/package/>
