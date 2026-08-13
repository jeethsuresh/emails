package mailmeta

import "testing"

func TestUpsertThreadAggregatesAllMessages(t *testing.T) {
	app := newMailmetaTestApp(t)
	older := saveTestRecord(t, app, "messages", map[string]any{
		"thread_id": "threadshared001",
		"subject":   "Project",
		"snippet":   "first",
		"from_addr": "Alice <alice@example.net>",
		"to_addrs":  "me@example.com",
		"date":      "2026-08-13T12:00:00Z",
		"folder":    "inbox",
		"seen":      false,
	})
	newer := saveTestRecord(t, app, "messages", map[string]any{
		"thread_id": "threadshared001",
		"subject":   "Re: Project",
		"snippet":   "reply",
		"from_addr": "me@example.com",
		"to_addrs":  "alice@example.net",
		"date":      "2026-08-13T13:00:00Z",
		"folder":    "sent",
		"seen":      true,
	})

	if err := UpsertThreadFromMessage(app, older); err != nil {
		t.Fatal(err)
	}
	thread, err := app.FindRecordById("threads", "threadshared001")
	if err != nil {
		t.Fatal(err)
	}
	if got := thread.GetFloat("message_count"); got != 2 {
		t.Fatalf("message_count = %v, want 2", got)
	}
	if got := thread.GetFloat("unread_count"); got != 1 {
		t.Fatalf("unread_count = %v, want 1", got)
	}
	// Newest message wins for the denormalized summary fields.
	if got := thread.GetString("last_date"); got != newer.GetString("date") {
		t.Fatalf("last_date = %q, want %q", got, newer.GetString("date"))
	}
	if got := thread.GetString("snippet"); got != "reply" {
		t.Fatalf("snippet = %q, want reply", got)
	}
	if got := thread.GetString("participants"); got != "alice@example.net, me@example.com" {
		t.Fatalf("participants = %q", got)
	}
}

func TestRecountThreadDeletesThreadWhenLastMessageMovesAway(t *testing.T) {
	app := newMailmetaTestApp(t)
	msg := saveTestRecord(t, app, "messages", map[string]any{
		"thread_id": "threadorphan001",
		"subject":   "Project",
		"date":      "2026-08-13T12:00:00Z",
	})
	if err := UpsertThreadFromMessage(app, msg); err != nil {
		t.Fatal(err)
	}

	msg.Set("thread_id", "threadmerged001")
	if err := app.Save(msg); err != nil {
		t.Fatal(err)
	}
	if err := RecountThread(app, "threadorphan001"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.FindRecordById("threads", "threadorphan001"); err == nil {
		t.Fatal("thread with no messages must be deleted")
	}
}

func TestRecountThreadUpdatesCountWhenOneMessageLeaves(t *testing.T) {
	app := newMailmetaTestApp(t)
	stays := saveTestRecord(t, app, "messages", map[string]any{
		"thread_id": "threadpartial01",
		"subject":   "Project",
		"snippet":   "first",
		"date":      "2026-08-13T12:00:00Z",
		"seen":      true,
	})
	leaves := saveTestRecord(t, app, "messages", map[string]any{
		"thread_id": "threadpartial01",
		"subject":   "Re: Project",
		"snippet":   "reply",
		"date":      "2026-08-13T13:00:00Z",
		"seen":      true,
	})
	if err := UpsertThreadFromMessage(app, stays); err != nil {
		t.Fatal(err)
	}

	leaves.Set("thread_id", "threadelsewhere")
	if err := app.Save(leaves); err != nil {
		t.Fatal(err)
	}
	if err := RecountThread(app, "threadpartial01"); err != nil {
		t.Fatal(err)
	}
	thread, err := app.FindRecordById("threads", "threadpartial01")
	if err != nil {
		t.Fatal(err)
	}
	if got := thread.GetFloat("message_count"); got != 1 {
		t.Fatalf("message_count = %v, want 1", got)
	}
	if got := thread.GetString("snippet"); got != "first" {
		t.Fatalf("snippet = %q, want first", got)
	}
}

func TestRecountThreadIgnoresUnknownThread(t *testing.T) {
	app := newMailmetaTestApp(t)
	if err := RecountThread(app, "threadmissing01"); err != nil {
		t.Fatal(err)
	}
	if err := RecountThread(app, "  "); err != nil {
		t.Fatal(err)
	}
}
