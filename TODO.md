# TODO — phase C & follow-ups

## Phase C (scaffolded, not shipped)

- [ ] Multi-account switching in UI + sync scheduler per account
- [ ] Contacts graph UI (collection `contacts.graph_json` already exists)
- [ ] Offline compose queue with durable SMTP worker and retry/backoff
- [ ] OAuth (Gmail / Outlook) with password fallback

## Hardening

- [ ] Persist memdb snapshots to disk after sync / on quit (host `saveDB` is wired; flush not scheduled yet)
- [ ] Encrypt account passwords at rest (OS keychain / PB secret)
- [ ] Attachment download + disk storage under `attachments/`
- [ ] Full-body IMAP fetch + multipart MIME via C parser
- [ ] Real FTS index in C (currently tokenize + PB filter)
- [ ] BLAKE3 (or similar) instead of FNV stand-in in `email_core`
- [ ] UtilityProcess isolation for WASM (keep UI snappy)
- [ ] SMTP send path wired from drafts
- [ ] Unit tests for C core + Go syncer (with netbridge fake)
