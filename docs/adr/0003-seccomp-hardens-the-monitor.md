# ADR 0003 — Seccomp hardens the monitor, not the guest

**Status:** Accepted · **Date:** 2026-05

## Context
It is tempting to present a seccomp profile as "restricting what the tool can do
inside its sandbox." For urunc that would be **wrong**. Per
<https://urunc.io/design/seccomp/>, seccomp filters apply to the **host-side
monitor process** (e.g. QEMU). The guest runs its own kernel and is isolated by
the hypervisor boundary regardless of any seccomp profile.

## Decision
- Ship `configs/seccomp/strict.json` as a **deny-dangerous** profile
  (`defaultAction: ALLOW` + explicit `ERRNO` for module loading, mount, ptrace,
  reboot, eBPF, keyring, etc.) and apply it to `code_tool`.
- Use `defaultAction: ALLOW` rather than an allowlist because an over-tight
  allowlist would break the QEMU monitor itself.
- Document everywhere (here, in the profile's `_comment`, in docs/URUNC.md §7)
  that this hardens the **host attack surface of the monitor**, not the guest.

## Consequences
- A real, defensible defense-in-depth control, described accurately.
- No false sense that seccomp sandboxes the code inside the VM — the VM boundary
  already does that; seccomp shrinks blast radius if the monitor is compromised.

## References
- urunc seccomp design: <https://urunc.io/design/seccomp/>
