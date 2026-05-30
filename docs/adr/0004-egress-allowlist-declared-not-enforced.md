# ADR 0004 — Egress allowlist is declared, not enforced

**Status:** Accepted · **Date:** 2026-05

## Context
A natural per-tool control is "this tool may only reach these destinations."
The policy schema has an `egress_allowlist` field and it is tempting to imply it
is enforced. It is not — and claiming otherwise would be a security lie a mentor
would (rightly) catch.

nerdctl/urunc expose only two network states: `--network=none` (no NIC) and
`--network=bridge` (**full, unfiltered egress**). There is no per-destination
filter flag. The urunc CNI bridge does not filter by domain or CIDR.

## Decision
- Keep `egress_allowlist` in the schema and surface it in `/api/v1/tools`, the
  audit log, and the dashboard — as **declared policy**.
- Mark it `egress_enforced: false` in the API and **log a warning** at execution
  time when a bridged tool declares an allowlist.
- Document the real enforcement paths for the future: a **chained CNI firewall
  plugin** (CIDR-only; domains must be pre-resolved) or an **egress proxy** the
  tool is forced through with direct egress dropped.

## Consequences
- The platform is honest about its boundary; the field is useful as intent and
  as a hook for future enforcement, without pretending to be a control today.

## References
- urunc network handling: <https://urunc.io/design/>
- Capability ledger: [docs/URUNC.md](../URUNC.md) §8
