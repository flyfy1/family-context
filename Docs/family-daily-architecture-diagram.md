# Family Daily Architecture Diagram

> Updated: 2026-08-22
> Scope: current implementation and staged future direction

## Status legend

- Green: implemented and locally verified
- Yellow: implemented prototype, still needs real-device or real-family proof
- Red: required before family trial
- Gray dashed: later direction, not part of the current MVP

## Current architecture

```mermaid
flowchart LR
    child[Adult child<br/>asks and replies]:::verified
    elder[Parent<br/>records and confirms]:::trial
    chat[Existing chat app<br/>manual link delivery]:::trial

    subgraph clients[Client layer]
        web[Responsive Web app<br/>embedded HTML CSS JS<br/>recording and family feed]:::verified
        android[Native Android prototype<br/>Java MediaRecorder AAC<br/>debug APK builds]:::trial
    end

    subgraph service[Single Go service]
        static[Embedded Web server]:::verified
        auth[Shared family-token guard<br/>temporary local identity]:::trial
        qa[Question Answer Reply APIs<br/>publish rerecord history]:::verified
        context[Member Update Daily Summary APIs<br/>implemented beyond current P1 UI]:::trial
        audio[Audio ingestion<br/>18 MB limit and atomic local write]:::verified
        audit[Append-only audit events<br/>archived rerecord revisions]:::verified
    end

    subgraph local[Authoritative local storage root]
        sqlite[(SQLite<br/>questions answers replies<br/>members updates summaries audit)]:::verified
        media[(Local media files<br/>original recordings)]:::verified
        spaces[(Member and shared spaces<br/>Markdown JSON and media mirror)]:::trial
        marker[Production mount marker<br/>fail closed when disk is absent]:::verified
        backup[Backups directory<br/>reserved only]:::required
    end

    gemini[Gemini Interactions API<br/>stateless audio processing<br/>store false]:::verified

    child --> web
    elder --> web
    elder --> android
    child --> chat --> elder
    web --> static
    android --> auth
    static --> auth
    auth --> qa
    auth --> context
    qa --> audio
    qa --> audit
    qa --> sqlite
    context --> sqlite
    context --> spaces
    audio --> media
    audio --> gemini
    gemini -->|transcript and conservative summary| qa
    marker -. startup gate .-> service
    sqlite -. not yet backed up .-> backup
    media -. not yet backed up .-> backup

    classDef verified fill:#DCFCE7,stroke:#15803D,color:#14532D,stroke-width:2px;
    classDef trial fill:#FEF3C7,stroke:#D97706,color:#78350F,stroke-width:2px;
    classDef required fill:#FEE2E2,stroke:#DC2626,color:#7F1D1D,stroke-width:2px;
    classDef later fill:#F3F4F6,stroke:#6B7280,color:#374151,stroke-dasharray:6 4;
```

## Future direction by learning stage

```mermaid
flowchart TB
    now[Now<br/>local Web PoC plus Android prototype]:::verified

    subgraph gate1[Next learning gate]
        device[Real Android phone recording<br/>upload playback publish reply]:::trial
        families[2 to 3 internal families<br/>complete the full loop]:::trial
        observe[Observe permission friction<br/>AI fidelity wait tolerance and replies]:::trial
    end

    subgraph gate2[Required before wider family trial]
        https[HTTPS entry point]:::required
        identity[Real users families members<br/>invite answer links expiry and revocation]:::required
        consent[Clear AI-processing consent]:::required
        delete[Complete user deletion<br/>including revisions media and sensitive audit payloads]:::required
        durable[Dedicated local disk configuration<br/>encrypted off-disk backup and restore drill]:::required
    end

    subgraph target[Target V1 deployment architecture]
        clients2[Mobile Web first<br/>Android after Web loop proves value]:::trial
        edge[HTTPS reverse proxy]:::required
        app2[One Go application<br/>UI API auth and AI orchestration]:::trial
        disk2[(Dedicated local disk<br/>SQLite media spaces and audit)]:::trial
        ai2[Gemini<br/>stateless request processing only]:::trial
        backup2[(Encrypted offline or off-disk backup)]:::required
    end

    subgraph later[Later only after usage evidence]
        reminders[Optional reminders or push]:::later
        daily[Family Daily feed and summaries]:::later
        threads[Topic continuity and family memory]:::later
        ops[Disk health capacity and operational alerts]:::later
    end

    nogo[Explicit non-direction<br/>No hosted authoritative database<br/>No object store queues NAS sync or replication<br/>until real usage requires a product decision]:::boundary

    now --> device --> families --> observe
    observe -->|value and usability validated| gate2
    gate2 --> target
    clients2 --> edge --> app2
    app2 --> disk2
    app2 --> ai2
    disk2 --> backup2
    target -->|only after V1 evidence| later
    nogo -. constrains .-> target
    nogo -. constrains .-> later

    classDef verified fill:#DCFCE7,stroke:#15803D,color:#14532D,stroke-width:2px;
    classDef trial fill:#FEF3C7,stroke:#D97706,color:#78350F,stroke-width:2px;
    classDef required fill:#FEE2E2,stroke:#DC2626,color:#7F1D1D,stroke-width:2px;
    classDef later fill:#F3F4F6,stroke:#6B7280,color:#374151,stroke-dasharray:6 4;
    classDef boundary fill:#EDE9FE,stroke:#7C3AED,color:#4C1D95,stroke-width:2px;
```

## Architectural boundary

The V1 architecture stays intentionally small: clients call one Go service; the service writes all authoritative family data to SQLite and local media files before or alongside stateless Gemini processing. The next architecture work is identity, consent, deletion, HTTPS, dedicated-disk setup, and recoverable backup—not distributed infrastructure.
