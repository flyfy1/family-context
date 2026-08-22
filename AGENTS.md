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

## Machine context

- Read the gitignored `.agent-machine.env` for this checkout's machine role and use `.agent-machine.env.example` only as a safe schema.
- The production frontend has one authoritative deployment: GitHub Pages at `https://family.integ.life`.
- The production backend runs on the Mac mini addressed by the deployment alias `mmini`, with Cloudflare Tunnel exposing `https://family-api.integ.life`.
- The Go backend must not embed or serve the React frontend. Its public root returns API metadata, while `/api`, `/mcp`, OAuth, media, health, and version endpoints remain backend-owned.
- Never store family, administrator, member, Gemini, Cloudflare, or other credentials in machine-context files.

## Work logs

- Every task performed in this repository must have a durable Integ.Life work log.
- Create the work log at the start of a new task and keep updating the same log until the task is complete; follow-up work on the same task must reuse that log.
- Name each work log using `YYYY-MM-DD-<task-name>`, with the calendar date followed by a short, specific task name.
- Record the goal, task-owned changes, verification evidence, blockers, and commit or push state. Never include credentials or other secrets.
- Sync the work log through the existing `life` CLI and confirm that the corresponding Note reports `synced: true`.

## Commits

- After completing and verifying each self-contained unit of change, commit that unit immediately before starting the next one. Do not wait until the end of the task to batch completed units into one commit.
- Stage and commit only the files or hunks owned by that unit. Preserve unrelated staged, unstaged, and untracked work.

## Deployment

- After development work is complete and verified, deploy the affected production surface immediately unless the user explicitly excludes deployment or a real deployment blocker prevents it.
- Frontend changes deploy only through the authoritative GitHub Pages workflow for `https://family.integ.life`; backend changes deploy only to the Mac mini through the established `mmini` release flow.
- Wait for the deployment process to finish naturally, then verify the relevant behavior on the real production URL. A successful build, push, workflow status, health response, or version endpoint alone is not sufficient production proof.
- Record the deployed commit, deployment result, and production verification evidence in the task's existing Integ.Life work log.
