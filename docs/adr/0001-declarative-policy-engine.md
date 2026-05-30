# ADR 0001 — Tools are declared in YAML, not hardcoded in Go

**Status:** Accepted · **Date:** 2026-05

## Context
The first version hardcoded four tools and their isolation profiles in Go
(`pkg/tool/registry.go`). Adding a tool meant editing source and recompiling,
and three functions (`RegisterAll`, `ProfileSummary`, `PrintProfile`) each
duplicated the literal list of four tools — a change had to be made in four
places and could drift.

## Decision
Tools become **data**. `configs/policies.yaml` declares any number of tools and
their full isolation profile; `pkg/policy/loader.go` parses, merges a `defaults`
block, validates, and builds the registry. The registry now preserves insertion
order, and the MCP/REST/CLI layers iterate `Registry.All()` so a new YAML tool
appears everywhere automatically. `NewRegistry()` remains an in-code fallback so
the binary still runs with no config present.

## Consequences
- Adding/tuning a tool is a YAML edit, no recompile — fast iteration, reviewable diffs.
- One source of truth; the three previously-duplicated functions now iterate the registry.
- Validation moves to load time (duplicate types, unknown monitor/network rejected).
- Trade-off: a YAML typo is a runtime (startup) error rather than a compile error;
  mitigated by `validate()` and the fallback to built-in defaults.

## References
- urunc annotations that the profile fields map to: <https://urunc.io/package/>
