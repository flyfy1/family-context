# Family Daily repository policy

## Server data

- The backend's authoritative state is local SQLite plus local media files under one configured storage root.
- Production must set `FAMILY_DAILY_STORAGE_DIR` to an absolute path on the dedicated local disk.
- Production must fail closed when `<storage-root>/.family-daily-storage` is missing. This prevents silently writing to the system disk when the dedicated disk is not mounted.
- Do not move authoritative family data to a hosted database or object store without an explicit product decision.
- Gemini may process an audio request, but requests must remain stateless and all durable source audio, transcripts, summaries, versions, and audit events remain local.
- User-facing removal and retention need explicit semantics. V1 draft re-recording archives the previous local revision for traceability; a future permanent-erasure flow must delete all corresponding local revisions and media.
- Each member Space is isolated under `spaces/members/<member-id>`. Member and MCP credentials must never authorize another member's private files.
- MCP file tools are confined to the authenticated member's `context/` directory. Do not expose the storage root, NAS mount, arbitrary filesystem paths, or shell execution through MCP.
- Production admin credentials must be distinct from family and member credentials. Member tokens are stored only as hashes and plaintext is returned only when issued or rotated.

## MVP boundary

- Preserve the V1 question → voice answer → AI organization → confirmation → reply loop.
- Do not add distributed storage, queues, sync, NAS dependencies, or replication before real family usage requires them.
- A mounted NAS may be the configured production storage root, but capacity is not a substitute for backups, mount validation, or recovery testing.

## Work logs

- Every task performed in this repository must have a durable Integ.Life work log.
- Create the work log at the start of a new task and keep updating the same log until the task is complete; follow-up work on the same task must reuse that log.
- Name each work log using `YYYY-MM-DD-<task-name>`, with the calendar date followed by a short, specific task name.
- Record the goal, task-owned changes, verification evidence, blockers, and commit or push state. Never include credentials or other secrets.
- Sync the work log through the existing `life` CLI and confirm that the corresponding Note reports `synced: true`.

## Commits

- After completing and verifying each self-contained unit of change, commit that unit immediately before starting the next one. Do not wait until the end of the task to batch completed units into one commit.
- Stage and commit only the files or hunks owned by that unit. Preserve unrelated staged, unstaged, and untracked work.
