# Send, Threads, Aliases, Contacts & Suggested Replies — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** SMTP send, conversation threads, receiving-alias filter, contacts, and AI suggested-reply → Compose — with **all durable logic in Go + PocketBase**; React is display + thin API calls only.

**Architecture:** New `backend/internal/mailmeta` (pure helpers: received_for, subject normalize, thread resolve) + `backend/internal/mailer` (SMTP + MIME) + `backend/internal/mailapi` (HTTP routes). Sync ingest writes thread/alias/contact fields; UI calls `/api/email/*` for lists/send/compose prefills.

**Tech Stack:** Go 1.24, PocketBase v0.31, `net/smtp` + existing `netbridge` TLS modes, React/Vite PocketBase client, existing analyzer `suggested_reply`.

## Global Constraints

- Spec: `docs/superpowers/specs/2026-08-13-send-threads-aliases-contacts-design.md`
- **No** client-side threading, alias parsing, or contact aggregation
- Reuse existing `contacts` collection (extend fields); do not create a second contacts table
- Alias headers: `Delivered-To`, `X-Original-To`, `X-Delivered-To`, `Envelope-To`, then To/Cc domain match
- Threading: `In-Reply-To`/`References` first; else normalized subject + participants (90-day window)
- Reply From defaults to message `received_for`
- Suggested reply is additive (does not replace Apply for folder/todo/event)
- Native backend only for SMTP (`cmd/native`); keep wasm builds compiling (stub mailer register if needed)
- Prefer `go test ./...` under `backend/` for Go; `npx tsc -p tsconfig.json --noEmit` for UI

---

## File map

| File | Role |
| --- | --- |
| `backend/internal/mailstore/collections.go` | Schema: message thread fields, `threads`, draft send fields, extend `contacts` |
| `backend/internal/mailmeta/normalize.go` | Email normalize, subject strip Re:/Fwd: |
| `backend/internal/mailmeta/received_for.go` | Alias detection from headers + To/Cc |
| `backend/internal/mailmeta/thread.go` | Resolve `thread_id` from Message-ID graph + fallback |
| `backend/internal/mailmeta/*_test.go` | Unit tests (no PB) |
| `backend/internal/mailer/smtp.go` | Build MIME + SMTP send with tls/starttls/none |
| `backend/internal/mailer/smtp_test.go` | MIME header tests; optional local fake if cheap |
| `backend/internal/mailapi/register.go` | Wire routes on `OnServe` |
| `backend/internal/mailapi/threads.go` | List/get threads |
| `backend/internal/mailapi/aliases.go` | Distinct `received_for` |
| `backend/internal/mailapi/contacts.go` | List contacts + messages-from |
| `backend/internal/mailapi/compose.go` | Reply prefill |
| `backend/internal/mailapi/send.go` | Send + persist Sent + draft status |
| `backend/internal/syncer/sync.go` | On upsert: headers → fields → thread → contact |
| `backend/internal/mailstore/backfill_mail.go` | One-shot backfill missing thread/alias/contacts |
| `backend/cmd/native/main.go` | `mailapi.Register(app)` |
| `src/lib/mailApi.ts` | Thin fetch wrappers for new endpoints |
| `src/components/ComposeModal.tsx` | From, Send, reply prefills |
| `src/components/MessageView.tsx` | Use reply + Reply button |
| `src/components/ThreadList.tsx` | Replace raw message list when viewing folder |
| `src/components/AliasFilter.tsx` | Chips from `/aliases` |
| `src/components/ContactsView.tsx` | Contacts + timeline |
| `src/App.tsx` / `MailShell.tsx` / `AppChrome.tsx` | Wire thread mode, alias filter, Contacts tab/entry |

---

### Task 1: PocketBase schema extensions

**Files:**
- Modify: `backend/internal/mailstore/collections.go`
- Create: `backend/internal/mailstore/schema_mail_test.go` (optional compile/smoke) — prefer calling ensure helpers via exported test of field presence if awkward; otherwise skip test and verify by starting app once in Task 4

**Interfaces:**
- Produces: message fields `in_reply_to`, `references`, `thread_id`, `received_for`, `normalized_subject`; collection `threads`; draft fields `from_addr`, `in_reply_to`, `references`, `thread_id`, `status`, `last_error`, `sent_at`, `message_id`; contacts fields `last_message_at`, `message_count`, `updated_at` (keep `email`, `display_name`, `graph_json`)

- [ ] **Step 1: Add `ensureMailFeatureSchema` chain**

At end of `ensureLLMAnalysisSchemaFields` (or after `ensureDefaultCalendarAndMigrateEvents`), call `ensureMailFeatureSchema(app)`.

```go
func ensureMailFeatureSchema(app core.App) error {
	if err := ensureMessageThreadFields(app); err != nil {
		return err
	}
	if err := ensureThreadsCollection(app); err != nil {
		return err
	}
	if err := ensureDraftSendFields(app); err != nil {
		return err
	}
	if err := ensureContactsMailFields(app); err != nil {
		return err
	}
	return ensureMailIndexes(app)
}

func ensureMessageThreadFields(app core.App) error {
	col, err := app.FindCollectionByNameOrId("messages")
	if err != nil {
		return err
	}
	changed := false
	add := func(f core.Field) {
		if col.Fields.GetByName(f.GetName()) == nil {
			col.Fields.Add(f)
			changed = true
		}
	}
	add(&core.TextField{Name: "in_reply_to"})
	add(&core.TextField{Name: "references", Max: 20_000})
	add(&core.TextField{Name: "thread_id"})
	add(&core.TextField{Name: "received_for"})
	add(&core.TextField{Name: "normalized_subject"})
	if !changed {
		return nil
	}
	return app.Save(col)
}

func ensureThreadsCollection(app core.App) error {
	if _, err := app.FindCollectionByNameOrId("threads"); err == nil {
		return nil
	}
	c := core.NewBaseCollection("threads")
	c.ListRule = types.Pointer("")
	c.ViewRule = types.Pointer("")
	c.CreateRule = types.Pointer("")
	c.UpdateRule = types.Pointer("")
	c.DeleteRule = types.Pointer("")
	c.Fields.Add(&core.TextField{Name: "subject"})
	c.Fields.Add(&core.TextField{Name: "normalized_subject"})
	c.Fields.Add(&core.TextField{Name: "snippet"})
	c.Fields.Add(&core.TextField{Name: "last_date"})
	c.Fields.Add(&core.NumberField{Name: "message_count"})
	c.Fields.Add(&core.TextField{Name: "participants", Max: 20_000})
	c.Fields.Add(&core.TextField{Name: "received_for"})
	c.Fields.Add(&core.TextField{Name: "folder"})
	c.Fields.Add(&core.NumberField{Name: "unread_count"})
	c.Fields.Add(&core.TextField{Name: "updated_at"})
	return app.Save(c)
}

func ensureDraftSendFields(app core.App) error {
	col, err := app.FindCollectionByNameOrId("drafts")
	if err != nil {
		return err
	}
	changed := false
	add := func(f core.Field) {
		if col.Fields.GetByName(f.GetName()) == nil {
			col.Fields.Add(f)
			changed = true
		}
	}
	add(&core.TextField{Name: "from_addr"})
	add(&core.TextField{Name: "in_reply_to"})
	add(&core.TextField{Name: "references", Max: 20_000})
	add(&core.TextField{Name: "thread_id"})
	add(&core.TextField{Name: "status"}) // draft | queued | sending | sent | failed
	add(&core.TextField{Name: "last_error", Max: 20_000})
	add(&core.TextField{Name: "sent_at"})
	add(&core.TextField{Name: "message_id"})
	if !changed {
		return nil
	}
	return app.Save(col)
}

func ensureContactsMailFields(app core.App) error {
	col, err := app.FindCollectionByNameOrId("contacts")
	if err != nil {
		return err
	}
	changed := false
	add := func(f core.Field) {
		if col.Fields.GetByName(f.GetName()) == nil {
			col.Fields.Add(f)
			changed = true
		}
	}
	add(&core.TextField{Name: "last_message_at"})
	add(&core.NumberField{Name: "message_count"})
	add(&core.TextField{Name: "updated_at"})
	if !changed {
		return nil
	}
	return app.Save(col)
}

func ensureMailIndexes(app core.App) error {
	msg, err := app.FindCollectionByNameOrId("messages")
	if err != nil {
		return err
	}
	addIdx := func(col *core.Collection, name, cols string) error {
		if col.GetIndex(name) != "" {
			return nil
		}
		col.AddIndex(name, false, cols, "")
		return app.Save(col)
	}
	if err := addIdx(msg, "idx_messages_thread_date", "`thread_id`,`date`"); err != nil {
		return err
	}
	if err := addIdx(msg, "idx_messages_received_for_date", "`received_for`,`date`"); err != nil {
		return err
	}
	if err := addIdx(msg, "idx_messages_from_date", "`from_addr`,`date`"); err != nil {
		return err
	}
	if err := addIdx(msg, "idx_messages_message_id", "`message_id`"); err != nil {
		return err
	}
	thr, err := app.FindCollectionByNameOrId("threads")
	if err != nil {
		return err
	}
	if err := addIdx(thr, "idx_threads_folder_last_date", "`folder`,`last_date`"); err != nil {
		return err
	}
	ct, err := app.FindCollectionByNameOrId("contacts")
	if err != nil {
		return err
	}
	if ct.GetIndex("idx_contacts_email") == "" {
		ct.AddIndex("idx_contacts_email", true, "`email`", "")
		if err := app.Save(ct); err != nil {
			return err
		}
	}
	return nil
}
```

Also add `threads` to the initial `defs` slice for fresh installs (same fields), and ensure `contacts` in defs already exists (do not duplicate).

- [ ] **Step 2: Commit**

```bash
git add backend/internal/mailstore/collections.go
git commit -m "feat(mailstore): schema for threads, aliases, send drafts, contacts"
```

---

### Task 2: Pure helpers — normalize + received_for

**Files:**
- Create: `backend/internal/mailmeta/normalize.go`
- Create: `backend/internal/mailmeta/received_for.go`
- Create: `backend/internal/mailmeta/normalize_test.go`
- Create: `backend/internal/mailmeta/received_for_test.go`

**Interfaces:**
- Produces:
  - `func NormalizeEmail(s string) string`
  - `func ExtractEmail(addr string) string` // `"Name <a@b>"` → `a@b`
  - `func NormalizeSubject(subject string) string`
  - `func DomainOf(email string) string`
  - `func ReceivedFor(headers map[string]string, toAddrs, ccAddrs, accountEmail string) string`
  - `func ParseAddressList(s string) []string`

- [ ] **Step 1: Write failing tests**

```go
package mailmeta

import "testing"

func TestNormalizeSubject(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Re: Hello", "hello"},
		{"RE: re: Fwd: Hello", "hello"},
		{"  Hello  ", "hello"},
		{"", ""},
	}
	for _, c := range cases {
		if got := NormalizeSubject(c.in); got != c.want {
			t.Fatalf("%q: got %q want %q", c.in, got, c.want)
		}
	}
}

func TestReceivedForPrefersDeliveredTo(t *testing.T) {
	h := map[string]string{
		"Delivered-To": "uber@jeeth.dev",
		"To":           "me@jeeth.dev",
	}
	got := ReceivedFor(h, "me@jeeth.dev", "", "me@jeeth.dev")
	if got != "uber@jeeth.dev" {
		t.Fatalf("got %q", got)
	}
}

func TestReceivedForFallsBackToDomainMatch(t *testing.T) {
	h := map[string]string{}
	got := ReceivedFor(h, "friend@gmail.com, uber@jeeth.dev", "", "me@jeeth.dev")
	if got != "uber@jeeth.dev" {
		t.Fatalf("got %q", got)
	}
}
```

- [ ] **Step 2: Run tests — expect FAIL**

```bash
cd backend && go test ./internal/mailmeta/ -count=1
```

Expected: FAIL (package/functions undefined)

- [ ] **Step 3: Implement**

```go
package mailmeta

import (
	"net/mail"
	"regexp"
	"strings"
)

var rePrefix = regexp.MustCompile(`(?i)^(re|fwd|fw)\s*:\s*`)

func NormalizeEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(ExtractEmail(s)))
}

func ExtractEmail(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}
	if a, err := mail.ParseAddress(addr); err == nil {
		return strings.ToLower(strings.TrimSpace(a.Address))
	}
	// bare email or angle-bracket scrapes
	if i := strings.LastIndex(addr, "<"); i >= 0 {
		j := strings.LastIndex(addr, ">")
		if j > i {
			return strings.ToLower(strings.TrimSpace(addr[i+1 : j]))
		}
	}
	return strings.ToLower(addr)
}

func ParseAddressList(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	list, err := mail.ParseAddressList(s)
	if err != nil {
		// split on commas best-effort
		parts := strings.Split(s, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			if e := NormalizeEmail(p); e != "" {
				out = append(out, e)
			}
		}
		return out
	}
	out := make([]string, 0, len(list))
	for _, a := range list {
		out = append(out, strings.ToLower(strings.TrimSpace(a.Address)))
	}
	return out
}

func DomainOf(email string) string {
	email = NormalizeEmail(email)
	i := strings.LastIndex(email, "@")
	if i < 0 {
		return ""
	}
	return email[i+1:]
}

func NormalizeSubject(subject string) string {
	s := strings.TrimSpace(strings.ToLower(subject))
	for {
		ns := strings.TrimSpace(rePrefix.ReplaceAllString(s, ""))
		if ns == s {
			return ns
		}
		s = ns
	}
}

func ReceivedFor(headers map[string]string, toAddrs, ccAddrs, accountEmail string) string {
	for _, key := range []string{"Delivered-To", "X-Original-To", "X-Delivered-To", "Envelope-To"} {
		if v := NormalizeEmail(headerGet(headers, key)); v != "" {
			return v
		}
	}
	domain := DomainOf(accountEmail)
	candidates := append(ParseAddressList(toAddrs), ParseAddressList(ccAddrs)...)
	for _, c := range candidates {
		if domain != "" && DomainOf(c) == domain {
			return c
		}
		if NormalizeEmail(c) == NormalizeEmail(accountEmail) {
			return c
		}
	}
	return ""
}

func headerGet(h map[string]string, name string) string {
	if v, ok := h[name]; ok {
		return v
	}
	for k, v := range h {
		if strings.EqualFold(k, name) {
			return v
		}
	}
	return ""
}
```

- [ ] **Step 4: Run tests — expect PASS**

```bash
cd backend && go test ./internal/mailmeta/ -count=1
```

- [ ] **Step 5: Commit**

```bash
git add backend/internal/mailmeta/
git commit -m "feat(mailmeta): normalize subject and detect received_for"
```

---

### Task 3: Thread ID resolution (pure + lookup interface)

**Files:**
- Create: `backend/internal/mailmeta/thread.go`
- Create: `backend/internal/mailmeta/thread_test.go`

**Interfaces:**
- Consumes: `NormalizeSubject`, `NormalizeEmail`, `ParseAddressList`
- Produces:
  - `type ThreadLookup interface { ByMessageID(id string) (threadID string, ok bool); BySubjectParticipants(normSubject string, participants []string, sinceRFC3339 string) (threadID string, ok bool) }`
  - `func ResolveThreadID(messageID, inReplyTo, references, subject, from string, toAddrs string, lookup ThreadLookup, now time.Time) string`
  - `func CollectMessageIDs(inReplyTo, references string) []string`
  - `func NewThreadID(messageID string) string` // hash of messageID or uuid if empty

- [ ] **Step 1: Write failing tests with fake lookup**

```go
package mailmeta

import (
	"testing"
	"time"
)

type mapLookup struct {
	byMID map[string]string
	bySub map[string]string
}

func (m mapLookup) ByMessageID(id string) (string, bool) {
	v, ok := m.byMID[NormalizeMessageID(id)]
	return v, ok
}
func (m mapLookup) BySubjectParticipants(subj string, _ []string, _ string) (string, bool) {
	v, ok := m.bySub[subj]
	return v, ok
}

func TestResolveThreadViaInReplyTo(t *testing.T) {
	lu := mapLookup{byMID: map[string]string{"<a@x>": "thr1"}}
	got := ResolveThreadID("<b@x>", "<a@x>", "", "Re: Hi", "a@b.com", "c@d.com", lu, time.Now())
	if got != "thr1" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveThreadFallbackSubject(t *testing.T) {
	lu := mapLookup{bySub: map[string]string{"hi": "thr2"}}
	got := ResolveThreadID("<c@x>", "", "", "Re: Hi", "a@b.com", "c@d.com", lu, time.Now())
	if got != "thr2" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveThreadNewRoot(t *testing.T) {
	lu := mapLookup{}
	got := ResolveThreadID("<new@x>", "", "", "Unique", "a@b.com", "c@d.com", lu, time.Now())
	if got == "" {
		t.Fatal("expected new id")
	}
}
```

Implement `NormalizeMessageID` to strip/lower angle brackets consistently.

- [ ] **Step 2: Run — FAIL; implement ResolveThreadID**

Algorithm:
1. For each ID in `CollectMessageIDs(inReplyTo, references)` (References first then In-Reply-To, or prefer In-Reply-To then References — use In-Reply-To first then References tokens), if `ByMessageID` hits, return that thread.
2. Else `BySubjectParticipants(NormalizeSubject(subject), participants, now.Add(-90*24*time.Hour).UTC().Format(time.RFC3339))`.
3. Else `NewThreadID(messageID)`.

```go
func NormalizeMessageID(id string) string {
	id = strings.TrimSpace(strings.ToLower(id))
	id = strings.TrimPrefix(id, "<")
	id = strings.TrimSuffix(id, ">")
	return id
}

func CollectMessageIDs(inReplyTo, references string) []string {
	var out []string
	seen := map[string]struct{}{}
	add := func(raw string) {
		for _, tok := range strings.FieldsFunc(raw, func(r rune) bool {
			return r == ' ' || r == ',' || r == '\t' || r == '\n' || r == '\r'
		}) {
			n := NormalizeMessageID(tok)
			if n == "" {
				continue
			}
			if _, ok := seen[n]; ok {
				continue
			}
			seen[n] = struct{}{}
			out = append(out, n)
		}
	}
	add(inReplyTo)
	add(references)
	return out
}

func NewThreadID(messageID string) string {
	n := NormalizeMessageID(messageID)
	if n == "" {
		return uuid.NewString()
	}
	return host.Hash("thread|" + n) // or crypto/sha1 hex; keep short stable id
}

func ResolveThreadID(...) string {
	for _, mid := range CollectMessageIDs(inReplyTo, references) {
		if tid, ok := lookup.ByMessageID(mid); ok && tid != "" {
			return tid
		}
	}
	subj := NormalizeSubject(subject)
	parts := append([]string{NormalizeEmail(from)}, ParseAddressList(toAddrs)...)
	since := now.UTC().Add(-90 * 24 * time.Hour).Format(time.RFC3339)
	if tid, ok := lookup.BySubjectParticipants(subj, parts, since); ok && tid != "" {
		return tid
	}
	return NewThreadID(messageID)
}
```

Avoid importing `host` if it creates wasm issues — use local FNV hash copy in mailmeta instead:

```go
func NewThreadID(messageID string) string {
	n := NormalizeMessageID(messageID)
	if n == "" {
		return fmt.Sprintf("t_%d", time.Now().UnixNano())
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte("thread|" + n))
	return fmt.Sprintf("%08x", h.Sum32())
}
```

- [ ] **Step 3: Tests PASS; commit**

```bash
cd backend && go test ./internal/mailmeta/ -count=1
git add backend/internal/mailmeta/
git commit -m "feat(mailmeta): resolve thread_id from headers and subject fallback"
```

---

### Task 4: Ingest wiring + thread materialize + contacts upsert + backfill

**Files:**
- Create: `backend/internal/mailmeta/pb_lookup.go` (PocketBase `ThreadLookup`)
- Create: `backend/internal/mailmeta/apply.go` (`ApplyMessageMeta(app, rec, headers, accountEmail)`)
- Create: `backend/internal/mailmeta/threads_upsert.go` (`UpsertThreadFromMessage`)
- Create: `backend/internal/mailmeta/contacts_upsert.go` (`UpsertContactFromMessage`)
- Create: `backend/internal/mailstore/backfill_mail.go`
- Modify: `backend/internal/syncer/sync.go` (after setting subject/from/to/message_id, extract In-Reply-To/References/delivery headers from raw, call `ApplyMessageMeta`)

**Interfaces:**
- Consumes: Task 2–3 helpers, PB app
- Produces: messages saved with thread fields; `threads` + `contacts` updated on each ingest

- [ ] **Step 1: Implement PB lookup**

```go
type PBLookup struct{ App core.App }

func (p PBLookup) ByMessageID(id string) (string, bool) {
	col, err := p.App.FindCollectionByNameOrId("messages")
	if err != nil {
		return "", false
	}
	// match normalized or raw forms
	rec, err := p.App.FindFirstRecordByFilter(col.Id, "message_id = {:id} || message_id = {:angle}", dbx.Params{
		"id": id, "angle": "<" + id + ">",
	})
	if err != nil {
		return "", false
	}
	tid := rec.GetString("thread_id")
	return tid, tid != ""
}

func (p PBLookup) BySubjectParticipants(subj string, participants []string, since string) (string, bool) {
	if subj == "" {
		return "", false
	}
	col, err := p.App.FindCollectionByNameOrId("messages")
	if err != nil {
		return "", false
	}
	rows, err := p.App.FindRecordsByFilter(col.Id,
		"normalized_subject = {:s} && date >= {:since} && thread_id != ''",
		"-date", 20, 0, dbx.Params{"s": subj, "since": since})
	if err != nil {
		return "", false
	}
	want := map[string]struct{}{}
	for _, p := range participants {
		want[NormalizeEmail(p)] = struct{}{}
	}
	for _, r := range rows {
		parts := append(ParseAddressList(r.GetString("from_addr")), ParseAddressList(r.GetString("to_addrs"))...)
		overlap := 0
		for _, x := range parts {
			if _, ok := want[NormalizeEmail(x)]; ok {
				overlap++
			}
		}
		if overlap >= 1 {
			return r.GetString("thread_id"), true
		}
	}
	return "", false
}
```

- [ ] **Step 2: `ApplyMessageMeta` + upserts**

```go
func ApplyMessageMeta(app core.App, rec *core.Record, headers map[string]string, accountEmail string) {
	inReply := headerGet(headers, "In-Reply-To")
	refs := headerGet(headers, "References")
	rec.Set("in_reply_to", inReply)
	rec.Set("references", refs)
	rec.Set("normalized_subject", NormalizeSubject(rec.GetString("subject")))
	rec.Set("received_for", ReceivedFor(headers, rec.GetString("to_addrs"), "", accountEmail))
	mid := rec.GetString("message_id")
	now := time.Now()
	if d := rec.GetString("date"); d != "" {
		if t, err := time.Parse(time.RFC3339, d); err == nil {
			now = t
		}
	}
	tid := ResolveThreadID(mid, inReply, refs, rec.GetString("subject"), rec.GetString("from_addr"), rec.GetString("to_addrs"), PBLookup{App: app}, now)
	rec.Set("thread_id", tid)
}

func UpsertThreadFromMessage(app core.App, msg *core.Record) error { /* find threads by id=thread_id or create; update last_date/count/snippet/participants/unread/folder/received_for/updated_at */ }

func UpsertContactFromMessage(app core.App, msg *core.Record) error {
	email := NormalizeEmail(msg.GetString("from_addr"))
	if email == "" {
		return nil
	}
	// skip if email is our account
	col, err := app.FindCollectionByNameOrId("contacts")
	...
	rec, err := app.FindFirstRecordByFilter(col.Id, "email = {:e}", dbx.Params{"e": email})
	if err != nil {
		rec = core.NewRecord(col)
		rec.Set("email", email)
		rec.Set("message_count", 1)
	} else {
		rec.Set("message_count", rec.GetFloat("message_count")+1)
	}
	// parse display name from from_addr if present
	rec.Set("last_message_at", msg.GetString("date"))
	rec.Set("updated_at", time.Now().UTC().Format(time.RFC3339))
	return app.Save(rec)
}
```

For threads record: PocketBase base collections use auto ids — **set custom id** to `thread_id` when creating (`rec.Id = tid` before Save) so `threads.id == messages.thread_id`.

- [ ] **Step 3: Hook sync.go**

Where raw headers are available (~lines 903–960), build `headers map[string]string` via `host.MimeHeaderGet` for: Message-ID, Subject, In-Reply-To, References, Delivered-To, X-Original-To, X-Delivered-To, Envelope-To, To, Cc.

Load account email once per sync. After `rec.Set(...)` fields and **before** `app.Save(rec)`:

```go
mailmeta.ApplyMessageMeta(app, rec, headers, accountEmail)
```

After successful Save:

```go
_ = mailmeta.UpsertThreadFromMessage(app, rec)
_ = mailmeta.UpsertContactFromMessage(app, rec)
```

- [ ] **Step 4: Backfill**

```go
// mailstore/backfill_mail.go
func BackfillMailMeta(app core.App) error {
	// find account email
	// iterate messages where thread_id = '' OR received_for = '' in batches of 100
	// rebuild headers map from stored fields only (in_reply_to may be empty for old rows):
	//   for old rows without stored headers, set received_for via To/Cc domain only; thread via subject fallback + message_id
	// upsert threads/contacts
}
```

Call from `ensureMailFeatureSchema` end (async goroutine OK to avoid blocking bootstrap — if async, log errors; prefer sync for correctness on first serve if mailbox is small).

- [ ] **Step 5: Manual smoke** — restart backend, sync, spot-check PocketBase admin or SQL that new messages have `thread_id` / `received_for`.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/mailmeta/ backend/internal/mailstore/backfill_mail.go backend/internal/syncer/sync.go
git commit -m "feat(sync): assign thread_id, received_for, and contacts on ingest"
```

---

### Task 5: SMTP mailer (Go)

**Files:**
- Create: `backend/internal/mailer/mime.go`
- Create: `backend/internal/mailer/smtp.go`
- Create: `backend/internal/mailer/mime_test.go`

**Interfaces:**
- Produces:
  - `type SendInput struct { From, To []string, Cc []string, Subject, BodyText, InReplyTo, References, MessageID string }`
  - `func BuildRFC822(in SendInput) ([]byte, string, error)` // returns bytes + Message-ID used
  - `func SendSMTP(account *core.Record, raw []byte, from string, to []string) error`

- [ ] **Step 1: MIME tests**

```go
func TestBuildRFC822IncludesReplyHeaders(t *testing.T) {
	raw, mid, err := BuildRFC822(SendInput{
		From: "uber@jeeth.dev", To: []string{"a@x.com"}, Subject: "Re: Hi",
		BodyText: "ok", InReplyTo: "<a@x>", References: "<a@x>",
	})
	if err != nil { t.Fatal(err) }
	s := string(raw)
	if !strings.Contains(s, "In-Reply-To:") { t.Fatal(s) }
	if !strings.Contains(s, "From: uber@jeeth.dev") { t.Fatal(s) }
	if mid == "" { t.Fatal("message-id") }
}
```

- [ ] **Step 2: Implement BuildRFC822 + SendSMTP**

Use `net/smtp`:
- Resolve security via `netbridge.ParseSecurity(account.GetString("smtp_security"), account.GetBool("smtp_tls"))`
- Dial with `netbridge.Dial` for tls/none; for starttls: plain dial then `smtp.NewClient` + `StartTLS(netbridge.TLSConfig(...))`
- AUTH PLAIN with username/password from account
- `client.Mail`, `Rcpt`, `Data`

Generate Message-ID: `"<" + uuid + "@" + DomainOf(from) + ">"`.

- [ ] **Step 3: Tests PASS; commit**

```bash
cd backend && go test ./internal/mailer/ -count=1
git add backend/internal/mailer/
git commit -m "feat(mailer): build MIME and send via SMTP"
```

---

### Task 6: HTTP APIs — threads, aliases, contacts, compose, send

**Files:**
- Create: `backend/internal/mailapi/register.go`
- Create: `backend/internal/mailapi/threads.go`
- Create: `backend/internal/mailapi/aliases.go`
- Create: `backend/internal/mailapi/contacts.go`
- Create: `backend/internal/mailapi/compose.go`
- Create: `backend/internal/mailapi/send.go`
- Modify: `backend/cmd/native/main.go` — `mailapi.Register(app)`

**Interfaces (JSON):**

| Method | Path | Query/Body |
| --- | --- | --- |
| GET | `/api/email/threads` | `folder`, `received_for`, `page`, `perPage` |
| GET | `/api/email/threads/{id}` | messages ordered by date |
| GET | `/api/email/aliases` | — |
| GET | `/api/email/contacts` | `q` optional |
| GET | `/api/email/contacts/{email}/messages` | page |
| POST | `/api/email/compose/reply` | `{ "messageId", "useSuggestedReply": bool }` |
| POST | `/api/email/send` | see spec |

- [ ] **Step 1: Register routes** (mirror `calendar.Register`)

```go
func Register(app core.App) {
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		e.Router.GET("/api/email/threads", handleListThreads)
		e.Router.GET("/api/email/threads/{id}", handleGetThread)
		e.Router.GET("/api/email/aliases", handleAliases)
		e.Router.GET("/api/email/contacts", handleContacts)
		e.Router.GET("/api/email/contacts/{email}/messages", handleContactMessages)
		e.Router.POST("/api/email/compose/reply", handleComposeReply)
		e.Router.POST("/api/email/send", handleSend)
		return e.Next()
	})
}
```

- [ ] **Step 2: `handleListThreads`**

If `received_for` set: query distinct `thread_id` from messages where `received_for = ?` (and optional folder), then load those thread rows ordered by `last_date` DESC. Else list `threads` filtered by `folder` sort `-last_date`.

- [ ] **Step 3: `handleAliases`**

```sql
SELECT received_for, COUNT(*) AS n FROM messages WHERE received_for != '' GROUP BY received_for ORDER BY n DESC
```
via `app.DB().NewQuery(...).All(&rows)`.

- [ ] **Step 4: Contacts handlers** — PB filter on `contacts`; messages `from_addr = email`.

- [ ] **Step 5: Compose reply**

Load message + optional analysis. Prefill:
- `from` = `received_for` or account email
- `to` = counterpart (from_addr if not us, else first to)
- `subject` = ensure `Re: ` + original (if not already)
- `bodyText` = quoted original OR `suggested_reply` if flag
- `inReplyTo` / `references` / `threadId` from message

- [ ] **Step 6: Send**

Validate account; `BuildRFC822`; `SendSMTP`; update draft status if `draftId`; create/update Sent folder message row with thread fields; `UpsertThreadFromMessage`; best-effort IMAP APPEND to Sent (reuse syncer dial helpers if accessible — if hard, skip APPEND in v1 but still persist local Sent and document).

Return `{ "ok": true, "messageId", "threadId" }` or 500 with error.

- [ ] **Step 7: Wire main.go; `go test ./...` and build backend**

```bash
cd backend && go test ./... -count=1
npm run build:backend
```

- [ ] **Step 8: Commit**

```bash
git add backend/internal/mailapi/ backend/cmd/native/main.go
git commit -m "feat(mailapi): threads, aliases, contacts, compose reply, and send"
```

---

### Task 7: Thin frontend — API client + Compose Send + Use reply

**Files:**
- Create: `src/lib/mailApi.ts`
- Modify: `src/components/ComposeModal.tsx`
- Modify: `src/components/MessageView.tsx`
- Modify: `src/App.tsx` (open compose with prefill; Reply / Use reply)

**Interfaces:**

```ts
// src/lib/mailApi.ts
const base = () => import.meta.env.VITE_PB_URL ?? "http://127.0.0.1:8090";

export async function listAliases(): Promise<{ email: string; count: number }[]> { ... }
export async function listThreads(opts: { folder?: string; received_for?: string; page?: number }): Promise<...> { ... }
export async function getThread(id: string): Promise<{ thread; messages }> { ... }
export async function listContacts(q?: string): Promise<...> { ... }
export async function contactMessages(email: string, page?: number): Promise<...> { ... }
export async function composeReply(messageId: string, useSuggestedReply?: boolean): Promise<ComposePrefill> { ... }
export async function sendMail(body: SendBody): Promise<{ messageId: string; threadId: string }> { ... }
```

- [ ] **Step 1: Implement `mailApi.ts` using `fetch` to backend (same origin/port as PB).**

- [ ] **Step 2: Expand ComposeModal**

Props: optional `prefill?: ComposePrefill`, `aliases?: string[]`.
Fields: From (select), To, Subject, Body.
Buttons: Save draft (PB), **Send** (`sendMail`).
On Send success: close + toast/callback.

- [ ] **Step 3: MessageView**

Show `suggested_reply` text when present; buttons **Use reply** and **Reply** calling `onComposeReply(useSuggested)`.
Keep existing Apply action independent.

- [ ] **Step 4: Wire App.tsx** to pass handlers / prefill state into ComposeModal.

- [ ] **Step 5: `npx tsc -p tsconfig.json --noEmit`; commit**

```bash
git add src/lib/mailApi.ts src/components/ComposeModal.tsx src/components/MessageView.tsx src/App.tsx
git commit -m "feat(ui): compose send and suggested-reply prefill via Go APIs"
```

---

### Task 8: Thin frontend — thread list, alias filter, contacts

**Files:**
- Create: `src/components/ThreadList.tsx`
- Create: `src/components/AliasFilter.tsx`
- Create: `src/components/ContactsView.tsx`
- Modify: `src/components/MailShell.tsx`, `src/App.tsx`, `src/components/AppChrome.tsx`, `src/styles.css`

- [ ] **Step 1: AliasFilter** — load `/aliases`, chips; `onChange(email | "")`.

- [ ] **Step 2: ThreadList** — load `/threads?folder=&received_for=`; click opens thread via `/threads/{id}` and shows messages in reading stack (reuse MessageView for selected message inside thread).

- [ ] **Step 3: Replace folder message virtual list path in App with ThreadList when not searching (keep messageCache path for search if needed, or filter threads by q later — v1: search can remain message-based).

- [ ] **Step 4: ContactsView** — list contacts; select → `contactMessages`; optional entry in chrome (Mail sub-mode or Todos-adjacent tab). Prefer a **Contacts** control in Mail chrome / settings-adjacent to avoid crowding bottom tabs — e.g. folder rail header button "Contacts".

- [ ] **Step 5: Styles** for chips, thread rows (subject, snippet, count, unread).

- [ ] **Step 6: Typecheck; manual smoke checklist; commit**

```bash
npx tsc -p tsconfig.json --noEmit
git add src/
git commit -m "feat(ui): thread list, alias filter, and contacts (display-only)"
```

---

### Task 9: End-to-end verification

- [ ] Restart `npm run dev` (rebuilds backend)
- [ ] Confirm new collections/fields in PB
- [ ] Sync mail → messages have `thread_id` / `received_for`; contacts grow
- [ ] Alias chips appear; filter changes thread list
- [ ] Open thread → ordered messages
- [ ] Reply → From = received_for; Send succeeds (real SMTP)
- [ ] Suggested reply → Use reply prefills body; Apply still works for other actions
- [ ] Contact → only that sender’s mail
- [ ] `cd backend && go test ./... -count=1`
- [ ] Final commit only if leftover fixes

---

## Spec coverage checklist

| Spec item | Task |
| --- | --- |
| Go/PB source of truth | All tasks |
| messages thread/alias fields | 1, 4 |
| threads collection | 1, 4, 6 |
| contacts upsert | 1, 4, 6 |
| drafts send fields | 1, 6 |
| received_for algorithm | 2, 4 |
| threading algorithm | 3, 4 |
| backfill | 4 |
| SMTP send | 5, 6 |
| compose/reply API | 6, 7 |
| suggested_reply Use reply | 7 |
| aliases API + UI | 6, 8 |
| threads API + UI | 6, 8 |
| contacts API + UI | 6, 8 |
| UI display-only | 7, 8 |

## Placeholder scan

No TBD steps; IMAP Sent APPEND is explicitly best-effort / skippable in Task 6 Step 6 if dial helpers are not easily shared — local Sent persist is required either way.
