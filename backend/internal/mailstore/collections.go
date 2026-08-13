package mailstore

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
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
			c.Fields.Add(&core.TextField{Name: "in_reply_to"})
			c.Fields.Add(&core.TextField{Name: "references", Max: 20_000})
			c.Fields.Add(&core.TextField{Name: "thread_id"})
			c.Fields.Add(&core.TextField{Name: "received_for"})
			c.Fields.Add(&core.TextField{Name: "normalized_subject"})
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
			c.Fields.Add(&core.TextField{Name: "from_addr"})
			c.Fields.Add(&core.TextField{Name: "in_reply_to"})
			c.Fields.Add(&core.TextField{Name: "references", Max: 20_000})
			c.Fields.Add(&core.TextField{Name: "thread_id"})
			c.Fields.Add(&core.TextField{Name: "status"})
			c.Fields.Add(&core.TextField{Name: "last_error", Max: 20_000})
			c.Fields.Add(&core.TextField{Name: "sent_at"})
			c.Fields.Add(&core.TextField{Name: "message_id"})
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
			c.Fields.Add(&core.TextField{Name: "last_message_at"})
			c.Fields.Add(&core.NumberField{Name: "message_count"})
			c.Fields.Add(&core.TextField{Name: "updated_at"})
			c.AddIndex("idx_contacts_email", true, "`email`", "")
		}},
		{"threads", func(c *core.Collection) {
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
			c.AddIndex("idx_threads_folder_last_date", false, "`folder`,`last_date`", "")
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
			c.Fields.Add(&core.TextField{Name: "display_timezone"})
		}},
		{"calendars", func(c *core.Collection) {
			c.Fields.Add(&core.TextField{Name: "name", Required: true})
			c.Fields.Add(&core.TextField{Name: "color"})
			c.Fields.Add(&core.TextField{Name: "timezone"})
			c.Fields.Add(&core.TextField{Name: "source"}) // local | ics | caldav
			c.Fields.Add(&core.BoolField{Name: "is_visible"})
			c.Fields.Add(&core.BoolField{Name: "is_default"})
			c.Fields.Add(&core.TextField{Name: "ics_url"})
			c.Fields.Add(&core.TextField{Name: "caldav_url"})
			c.Fields.Add(&core.TextField{Name: "caldav_username"})
			c.Fields.Add(&core.TextField{Name: "caldav_secret"})
			c.Fields.Add(&core.TextField{Name: "caldav_calendar_path"})
			c.Fields.Add(&core.TextField{Name: "sync_token"})
			c.Fields.Add(&core.TextField{Name: "last_sync_at"})
			c.Fields.Add(&core.TextField{Name: "last_error"})
		}},
		{"events", func(c *core.Collection) {
			c.Fields.Add(&core.TextField{Name: "title"})
			c.Fields.Add(&core.TextField{Name: "notes", Max: 20_000})
			c.Fields.Add(&core.TextField{Name: "source_message"})
			c.Fields.Add(&core.TextField{Name: "created_at"})
			c.Fields.Add(&core.TextField{Name: "starts_at"})
			c.Fields.Add(&core.TextField{Name: "ends_at"})
			c.Fields.Add(&core.TextField{Name: "status"}) // draft | approved
			c.Fields.Add(&core.TextField{Name: "calendar"})
			c.Fields.Add(&core.BoolField{Name: "all_day"})
			c.Fields.Add(&core.TextField{Name: "timezone"})
			c.Fields.Add(&core.TextField{Name: "uid"})
			c.Fields.Add(&core.TextField{Name: "etag"})
			c.Fields.Add(&core.TextField{Name: "rrule"})
			c.Fields.Add(&core.TextField{Name: "exdate"})
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
		&core.TextField{Name: "display_timezone"},
	}, nil); err != nil {
		return err
	}
	if err := ensure("calendars", []core.Field{
		&core.TextField{Name: "name", Required: true},
		&core.TextField{Name: "color"},
		&core.TextField{Name: "timezone"},
		&core.TextField{Name: "source"},
		&core.BoolField{Name: "is_visible"},
		&core.BoolField{Name: "is_default"},
		&core.TextField{Name: "ics_url"},
		&core.TextField{Name: "caldav_url"},
		&core.TextField{Name: "caldav_username"},
		&core.TextField{Name: "caldav_secret"},
		&core.TextField{Name: "caldav_calendar_path"},
		&core.TextField{Name: "sync_token"},
		&core.TextField{Name: "last_sync_at"},
		&core.TextField{Name: "last_error"},
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
		&core.TextField{Name: "calendar"},
		&core.BoolField{Name: "all_day"},
		&core.TextField{Name: "timezone"},
		&core.TextField{Name: "uid"},
		&core.TextField{Name: "etag"},
		&core.TextField{Name: "rrule"},
		&core.TextField{Name: "exdate"},
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
	if err := backfillDraftStatusApproved(app); err != nil {
		return err
	}
	return ensureDefaultCalendarAndMigrateEvents(app)
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

const (
	DefaultCalendarName     = "Personal"
	DefaultCalendarColor    = "#0f6e56" // pine
	DefaultCalendarTimezone = "America/New_York"
	CalendarSourceLocal     = "local"
)

// FindDefaultCalendar returns the default calendar, preferring is_default,
// then any local calendar, then the first calendar row.
func FindDefaultCalendar(app core.App) (*core.Record, error) {
	col, err := app.FindCollectionByNameOrId("calendars")
	if err != nil {
		return nil, err
	}
	if rec, err := app.FindFirstRecordByFilter(col.Id, "is_default = true", nil); err == nil {
		return rec, nil
	}
	if rec, err := app.FindFirstRecordByFilter(col.Id, "source = {:s}", dbx.Params{"s": CalendarSourceLocal}); err == nil {
		return rec, nil
	}
	return app.FindFirstRecordByFilter(col.Id, "id != ''", nil)
}

// ensureDefaultCalendarAndMigrateEvents creates Personal if no calendars exist,
// then backfills events.calendar / all_day / timezone / uid.
func ensureDefaultCalendarAndMigrateEvents(app core.App) error {
	calCol, err := app.FindCollectionByNameOrId("calendars")
	if err != nil {
		return err
	}
	cals, err := app.FindRecordsByFilter(calCol.Id, "id != ''", "", 0, 0, nil)
	if err != nil {
		return err
	}
	if len(cals) == 0 {
		rec := core.NewRecord(calCol)
		rec.Set("name", DefaultCalendarName)
		rec.Set("color", DefaultCalendarColor)
		rec.Set("timezone", DefaultCalendarTimezone)
		rec.Set("source", CalendarSourceLocal)
		rec.Set("is_visible", true)
		rec.Set("is_default", true)
		if err := app.Save(rec); err != nil {
			return fmt.Errorf("create default calendar: %w", err)
		}
		cals = []*core.Record{rec}
	}

	def := cals[0]
	for _, c := range cals {
		if c.GetBool("is_default") {
			def = c
			break
		}
	}
	if !def.GetBool("is_default") {
		def.Set("is_default", true)
		if err := app.Save(def); err != nil {
			return err
		}
	}

	// New bool field defaults to false in SQLite; if every calendar is hidden,
	// treat as schema backfill and show them all once.
	allHidden := true
	for _, c := range cals {
		if c.GetBool("is_visible") {
			allHidden = false
			break
		}
	}
	if allHidden {
		for _, c := range cals {
			c.Set("is_visible", true)
			if err := app.Save(c); err != nil {
				return err
			}
		}
	}

	evCol, err := app.FindCollectionByNameOrId("events")
	if err != nil {
		return err
	}
	events, err := app.FindRecordsByFilter(evCol.Id, "id != ''", "", 0, 0, nil)
	if err != nil {
		return err
	}
	for _, ev := range events {
		changed := false
		if strings.TrimSpace(ev.GetString("calendar")) == "" {
			ev.Set("calendar", def.Id)
			changed = true
		}
		if strings.TrimSpace(ev.GetString("timezone")) == "" {
			ev.Set("timezone", "UTC")
			changed = true
		}
		if strings.TrimSpace(ev.GetString("uid")) == "" {
			ev.Set("uid", uuid.NewString())
			changed = true
		}
		// Legacy rows predate all_day; persist explicit false on first migrate.
		if changed {
			ev.Set("all_day", ev.GetBool("all_day"))
			if err := app.Save(ev); err != nil {
				return fmt.Errorf("migrate event %s: %w", ev.Id, err)
			}
		}
	}
	return ensureMailFeatureSchema(app)
}

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
