# Architecture docs

Index of architecture-level decision documents for AWP. Each doc pins a design decision with concrete examples and cross-references the goal contract / DAG that produced it.

| Doc                                              | Goal contract       | Status   | Scope                                                                                          |
|--------------------------------------------------|---------------------|----------|------------------------------------------------------------------------------------------------|
| [session-jsonl-alignment.md](./session-jsonl-alignment.md) | GC-2026-001 (DAG-2026-001) | Phase 1 design committed | Target on-disk schema + wire format for session JSONL, UUID v7 message IDs, custom/custom_message extension points, compaction deferral blueprint. Anchors P2-P6 of DAG-2026-001. |

## Conventions

- Decision docs are written before the code they describe. They pin the target shape with concrete JSON examples and explicitly note what is deferred to a later phase.
- Cross-references use the canonical goal/DAG path under `.pi/orchestrator/`.
- Terse, concrete, with examples. No filler.
- One doc per cross-cutting decision (e.g., one doc for "schema + wire format", another for "skill system" if that goal lands).

## Adding a new doc

1. Create the file under `docs/architecture/` with a slug matching the decision scope.
2. Link it from this README with the goal contract ID + status.
3. Cross-reference the goal contract at the top of the doc so future readers can trace design → code.
