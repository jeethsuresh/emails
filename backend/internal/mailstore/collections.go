package mailstore

import (
	"fmt"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

func Register(app core.App) {
	app.OnBootstrap().BindFunc(func(e *core.BootstrapEvent) error {
		if err := e.Next(); err != nil {
			return err
		}
		return ensureCollections(e.App)
	})
}

func ensureCollections(app core.App) error {
	defs := []struct {
		name   string
		fields func(*core.Collection)
	}{
		{"accounts", func(c *core.Collection) {
			c.Fields.Add(&core.TextField{Name: "email", Required: true})
			c.Fields.Add(&core.TextField{Name: "username", Required: true})
			c.Fields.Add(&core.TextField{Name: "password", Required: true})
			c.Fields.Add(&core.TextField{Name: "imap_host", Required: true})
			c.Fields.Add(&core.NumberField{Name: "imap_port", Required: true})
			c.Fields.Add(&core.BoolField{Name: "imap_tls"})
			c.Fields.Add(&core.TextField{Name: "imap_security"}) // tls | starttls | none
			c.Fields.Add(&core.TextField{Name: "smtp_host", Required: true})
			c.Fields.Add(&core.NumberField{Name: "smtp_port", Required: true})
			c.Fields.Add(&core.BoolField{Name: "smtp_tls"})
			c.Fields.Add(&core.TextField{Name: "smtp_security"})
			c.Fields.Add(&core.BoolField{Name: "tls_insecure"})
		}},
		{"folders", func(c *core.Collection) {
			c.Fields.Add(&core.TextField{Name: "account", Required: true})
			c.Fields.Add(&core.TextField{Name: "name", Required: true})
			c.Fields.Add(&core.TextField{Name: "role"})
			c.Fields.Add(&core.NumberField{Name: "uidvalidity"})
			c.Fields.Add(&core.NumberField{Name: "uidnext"})
			c.Fields.Add(&core.NumberField{Name: "sync_uid_max"})      // highest UID indexed
			c.Fields.Add(&core.NumberField{Name: "sync_backfill_uid"}) // next older UID boundary (exclusive high)
			c.Fields.Add(&core.BoolField{Name: "sync_complete"})
		}},
		{"messages", func(c *core.Collection) {
			c.Fields.Add(&core.TextField{Name: "account", Required: true})
			c.Fields.Add(&core.TextField{Name: "folder", Required: true})
			c.Fields.Add(&core.NumberField{Name: "uid", Required: true})
			c.Fields.Add(&core.TextField{Name: "message_id"})
			c.Fields.Add(&core.TextField{Name: "subject"})
			c.Fields.Add(&core.TextField{Name: "from_addr"})
			c.Fields.Add(&core.TextField{Name: "to_addrs"})
			c.Fields.Add(&core.TextField{Name: "date"})
			c.Fields.Add(&core.TextField{Name: "snippet"})
			c.Fields.Add(&core.TextField{Name: "body_text", Max: 2_000_000})
			c.Fields.Add(&core.TextField{Name: "body_html", Max: 2_000_000})
			c.Fields.Add(&core.BoolField{Name: "seen"})
			c.Fields.Add(&core.BoolField{Name: "flagged"})
			c.Fields.Add(&core.TextField{Name: "search_tokens", Max: 100_000})
			c.Fields.Add(&core.TextField{Name: "content_hash"})
		}},
		{"attachments", func(c *core.Collection) {
			c.Fields.Add(&core.TextField{Name: "message", Required: true})
			c.Fields.Add(&core.TextField{Name: "filename"})
			c.Fields.Add(&core.TextField{Name: "mime"})
			c.Fields.Add(&core.NumberField{Name: "size"})
			c.Fields.Add(&core.TextField{Name: "path"})
		}},
		{"drafts", func(c *core.Collection) {
			c.Fields.Add(&core.TextField{Name: "account", Required: true})
			c.Fields.Add(&core.TextField{Name: "to_addrs"})
			c.Fields.Add(&core.TextField{Name: "cc_addrs"})
			c.Fields.Add(&core.TextField{Name: "subject"})
			c.Fields.Add(&core.TextField{Name: "body_text", Max: 2_000_000})
			c.Fields.Add(&core.TextField{Name: "body_html", Max: 2_000_000})
		}},
		{"sync_meta", func(c *core.Collection) {
			c.Fields.Add(&core.TextField{Name: "account", Required: true})
			c.Fields.Add(&core.TextField{Name: "last_sync_at"})
			c.Fields.Add(&core.TextField{Name: "last_error"})
			c.Fields.Add(&core.NumberField{Name: "folders_synced"})
			c.Fields.Add(&core.NumberField{Name: "messages_synced"})
		}},
		{"contacts", func(c *core.Collection) {
			c.Fields.Add(&core.TextField{Name: "email", Required: true})
			c.Fields.Add(&core.TextField{Name: "display_name"})
			c.Fields.Add(&core.TextField{Name: "graph_json"})
		}},
		{"message_analysis", func(c *core.Collection) {
			c.Fields.Add(&core.TextField{Name: "message", Required: true})
			c.Fields.Add(&core.TextField{Name: "status"})
			c.Fields.Add(&core.TextField{Name: "priority"})
			c.Fields.Add(&core.TextField{Name: "suggested_action"})
			c.Fields.Add(&core.TextField{Name: "action_target"})
			c.Fields.Add(&core.TextField{Name: "suggested_reply", Max: 100_000})
			c.Fields.Add(&core.TextField{Name: "model"})
			c.Fields.Add(&core.TextField{Name: "error"})
			c.Fields.Add(&core.NumberField{Name: "fail_count"})
			c.Fields.Add(&core.TextField{Name: "analyzed_at"})
			c.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
			c.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
			c.AddIndex(messageAnalysisMessageUniqueIndex, true, "`message`", "")
		}},
		{"app_settings", func(c *core.Collection) {
			c.Fields.Add(&core.TextField{Name: "llm_model"})
			c.Fields.Add(&core.TextField{Name: "llm_base_url"})
			c.Fields.Add(&core.NumberField{Name: "sync_interval_minutes"})
		}},
		{"events", func(c *core.Collection) {
			c.Fields.Add(&core.TextField{Name: "title"})
			c.Fields.Add(&core.TextField{Name: "notes", Max: 20_000})
			c.Fields.Add(&core.TextField{Name: "source_message"})
			c.Fields.Add(&core.TextField{Name: "created_at"})
			c.Fields.Add(&core.TextField{Name: "starts_at"})
			c.Fields.Add(&core.TextField{Name: "ends_at"})
			c.Fields.Add(&core.TextField{Name: "status"}) // draft | approved
		}},
		{"todos", func(c *core.Collection) {
			c.Fields.Add(&core.TextField{Name: "title"})
			c.Fields.Add(&core.TextField{Name: "notes", Max: 20_000})
			c.Fields.Add(&core.TextField{Name: "source_message"})
			c.Fields.Add(&core.TextField{Name: "created_at"})
			c.Fields.Add(&core.TextField{Name: "deadline"})
			c.Fields.Add(&core.TextField{Name: "status"}) // draft | approved
		}},
	}

	for _, d := range defs {
		if _, err := app.FindCollectionByNameOrId(d.name); err == nil {
			continue
		}
		c := core.NewBaseCollection(d.name)
		c.ListRule = types.Pointer("")
		c.ViewRule = types.Pointer("")
		c.CreateRule = types.Pointer("")
		c.UpdateRule = types.Pointer("")
		c.DeleteRule = types.Pointer("")
		d.fields(c)
		if err := app.Save(c); err != nil {
			return fmt.Errorf("create collection %s: %w", d.name, err)
		}
	}
	return ensureAccountSecurityFields(app)
}

func ensureAccountSecurityFields(app core.App) error {
	col, err := app.FindCollectionByNameOrId("accounts")
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
	add(&core.TextField{Name: "imap_security"})
	add(&core.TextField{Name: "smtp_security"})
	add(&core.BoolField{Name: "tls_insecure"})
	if changed {
		if err := app.Save(col); err != nil {
			return err
		}
	}
	return ensureFolderSyncFields(app)
}

func ensureFolderSyncFields(app core.App) error {
	col, err := app.FindCollectionByNameOrId("folders")
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
	add(&core.NumberField{Name: "sync_uid_max"})
	add(&core.NumberField{Name: "sync_backfill_uid"})
	add(&core.BoolField{Name: "sync_complete"})
	if !changed {
		return ensureMessageBodyLimits(app)
	}
	if err := app.Save(col); err != nil {
		return err
	}
	return ensureMessageBodyLimits(app)
}

func ensureMessageBodyLimits(app core.App) error {
	bump := func(collection string, fields map[string]int) error {
		col, err := app.FindCollectionByNameOrId(collection)
		if err != nil {
			return err
		}
		changed := false
		for name, max := range fields {
			f := col.Fields.GetByName(name)
			tf, ok := f.(*core.TextField)
			if !ok || tf == nil {
				continue
			}
			if tf.Max < max {
				tf.Max = max
				changed = true
			}
		}
		if !changed {
			return nil
		}
		return app.Save(col)
	}
	if err := bump("messages", map[string]int{
		"body_text":     2_000_000,
		"body_html":     2_000_000,
		"search_tokens": 100_000,
	}); err != nil {
		return err
	}
	if err := bump("drafts", map[string]int{
		"body_text": 2_000_000,
		"body_html": 2_000_000,
	}); err != nil {
		return err
	}
	return ensureLLMAnalysisSchemaFields(app)
}

// messageAnalysisMessageUniqueIndex names the unique index enforcing at most
// one message_analysis row per message. PocketBase v0.31's core.TextField has
// no per-field Unique flag, so uniqueness is expressed as a collection index
// instead (see core.Collection.AddIndex).
const messageAnalysisMessageUniqueIndex = "idx_message_analysis_message"

func ensureLLMAnalysisSchemaFields(app core.App) error {
	ensure := func(name string, fields []core.Field, maxes map[string]int) error {
		col, err := app.FindCollectionByNameOrId(name)
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
		for _, f := range fields {
			add(f)
		}
		for fieldName, max := range maxes {
			f := col.Fields.GetByName(fieldName)
			tf, ok := f.(*core.TextField)
			if !ok || tf == nil {
				continue
			}
			if tf.Max < max {
				tf.Max = max
				changed = true
			}
		}
		if !changed {
			return nil
		}
		return app.Save(col)
	}
	if err := ensure("message_analysis", []core.Field{
		&core.TextField{Name: "message", Required: true},
		&core.TextField{Name: "status"},
		&core.TextField{Name: "priority"},
		&core.TextField{Name: "suggested_action"},
		&core.TextField{Name: "action_target"},
		&core.TextField{Name: "suggested_reply", Max: 100_000},
		&core.TextField{Name: "model"},
		&core.TextField{Name: "error"},
		&core.NumberField{Name: "fail_count"},
		&core.TextField{Name: "analyzed_at"},
		&core.AutodateField{Name: "created", OnCreate: true},
		&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
	}, map[string]int{"suggested_reply": 100_000}); err != nil {
		return err
	}
	if err := ensureMessageAnalysisUniqueIndex(app); err != nil {
		return err
	}
	if err := ensure("app_settings", []core.Field{
		&core.TextField{Name: "llm_model"},
		&core.TextField{Name: "llm_base_url"},
		&core.NumberField{Name: "sync_interval_minutes"},
	}, nil); err != nil {
		return err
	}
	if err := ensure("events", []core.Field{
		&core.TextField{Name: "title"},
		&core.TextField{Name: "notes", Max: 20_000},
		&core.TextField{Name: "source_message"},
		&core.TextField{Name: "created_at"},
		&core.TextField{Name: "starts_at"},
		&core.TextField{Name: "ends_at"},
		&core.TextField{Name: "status"},
	}, map[string]int{"notes": 20_000}); err != nil {
		return err
	}
	if err := ensure("todos", []core.Field{
		&core.TextField{Name: "title"},
		&core.TextField{Name: "notes", Max: 20_000},
		&core.TextField{Name: "source_message"},
		&core.TextField{Name: "created_at"},
		&core.TextField{Name: "deadline"},
		&core.TextField{Name: "status"},
	}, map[string]int{"notes": 20_000}); err != nil {
		return err
	}
	return backfillDraftStatusApproved(app)
}

func ensureMessagesListIndex(app core.App) error {
	col, err := app.FindCollectionByNameOrId("messages")
	if err != nil {
		return err
	}
	const name = "idx_messages_folder_date_uid"
	if col.GetIndex(name) != "" {
		return nil
	}
	col.AddIndex(name, false, "`folder`,`date`,`uid`", "")
	return app.Save(col)
}

// backfillDraftStatusApproved sets status='approved' on existing todos/events
// that predate the draft/approved distinction (empty or null status).
func backfillDraftStatusApproved(app core.App) error {
	if err := ensureMessagesListIndex(app); err != nil {
		return err
	}
	for _, table := range []string{"todos", "events"} {
		if _, err := app.DB().NewQuery(fmt.Sprintf(
			`UPDATE %s SET status = 'approved' WHERE status IS NULL OR status = ''`,
			table,
		)).Execute(); err != nil {
			return fmt.Errorf("backfill %s.status: %w", table, err)
		}
	}
	return nil
}

// ensureMessageAnalysisUniqueIndex adds the unique index on message_analysis.message
// if missing, guarding against duplicate analysis rows for the same message
// (belt-and-suspenders alongside the analyzer's enqueue-time check).
func ensureMessageAnalysisUniqueIndex(app core.App) error {
	col, err := app.FindCollectionByNameOrId("message_analysis")
	if err != nil {
		return err
	}
	if col.GetIndex(messageAnalysisMessageUniqueIndex) != "" {
		return nil
	}
	col.AddIndex(messageAnalysisMessageUniqueIndex, true, "`message`", "")
	return app.Save(col)
}

func UpsertAccount(app core.App, m map[string]any) error {
	col, err := app.FindCollectionByNameOrId("accounts")
	if err != nil {
		return err
	}
	email, _ := m["email"].(string)
	email = strings.TrimSpace(email)
	if email == "" {
		return fmt.Errorf("email required")
	}

	rec, err := app.FindFirstRecordByFilter(col.Id, "email = {:email}", dbx.Params{"email": email})
	if err != nil {
		rec = core.NewRecord(col)
	}
	set := func(k string, v any) { rec.Set(k, v) }
	set("email", email)
	set("username", str(m, "username", email))
	set("password", str(m, "password", ""))
	set("imap_host", str(m, "imapHost", str(m, "imap_host", "")))
	set("imap_port", num(m, "imapPort", num(m, "imap_port", 993)))
	imapSec := str(m, "imapSecurity", str(m, "imap_security", ""))
	if imapSec == "" {
		if boolVal(m, "imapTLS", boolVal(m, "imap_tls", true)) {
			imapSec = "tls"
		} else {
			imapSec = "none"
		}
	}
	set("imap_security", imapSec)
	set("imap_tls", imapSec == "tls")
	set("smtp_host", str(m, "smtpHost", str(m, "smtp_host", "")))
	set("smtp_port", num(m, "smtpPort", num(m, "smtp_port", 465)))
	smtpSec := str(m, "smtpSecurity", str(m, "smtp_security", ""))
	if smtpSec == "" {
		if boolVal(m, "smtpTLS", boolVal(m, "smtp_tls", true)) {
			smtpSec = "tls"
		} else {
			smtpSec = "none"
		}
	}
	set("smtp_security", smtpSec)
	set("smtp_tls", smtpSec == "tls")
	set("tls_insecure", boolVal(m, "tlsInsecure", boolVal(m, "tls_insecure", false)))
	return app.Save(rec)
}

func str(m map[string]any, k, def string) string {
	if v, ok := m[k].(string); ok {
		return v
	}
	return def
}

func num(m map[string]any, k string, def float64) float64 {
	switch v := m[k].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	default:
		return def
	}
}

func boolVal(m map[string]any, k string, def bool) bool {
	if v, ok := m[k].(bool); ok {
		return v
	}
	return def
}
