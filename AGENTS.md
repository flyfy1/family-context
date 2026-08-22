# Family Daily repository policy

## Server data

- The backend's authoritative state is local SQLite plus local media files under one configured storage root.
- Production must set `FAMILY_DAILY_STORAGE_DIR` to an absolute path on the dedicated local disk.
- Production must fail closed when `<storage-root>/.family-daily-storage` is missing. This prevents silently writing to the system disk when the dedicated disk is not mounted.
- Do not move authoritative family data to a hosted database or object store without an explicit product decision.
- Gemini may process an audio request, but requests must remain stateless and all durable source audio, transcripts, summaries, versions, and audit events remain local.
- User-facing removal and retention need explicit semantics. V1 draft re-recording archives the previous local revision for traceability; a future permanent-erasure flow must delete all corresponding local revisions and media.

## MVP boundary

- Preserve the V1 question → voice answer → AI organization → confirmation → reply loop.
- Do not add distributed storage, queues, sync, NAS dependencies, or replication before real family usage requires them.

