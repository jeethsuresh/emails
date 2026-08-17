package analyzer

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"email.local/backend/internal/calendar"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

const (
	pauseInterval      = 3 * time.Second
	idleInterval       = 1 * time.Second
	sweepBatchSize     = 100
	maxSweepBatches    = 10_000 // hard safety cap; ~1M messages worst case
	maxFailCount       = 3
	// Prefer the full message body. Only clip extremely large HTML-derived
	// text, keeping head + tail so itinerary/details near the end survive.
	maxPromptBodyChars = 200_000
	promptBodyHeadKeep = 140_000
	promptBodyTailKeep = 50_000
)

// wakeCh lets Enqueue/settings updates nudge a sleeping worker without
// blocking the caller — sends are best-effort (buffered, non-blocking).
var wakeCh = make(chan struct{}, 1)

func wake() {
	select {
	case wakeCh <- struct{}{}:
	default:
	}
}

func sleepOrWake(d time.Duration) {
	select {
	case <-time.After(d):
	case <-wakeCh:
	}
}

// Enqueue schedules a message for analysis. It never blocks the caller (e.g.
// the IMAP sync goroutine) — the actual DB upsert happens on its own
// goroutine and the worker is nudged via a non-blocking channel send.
func Enqueue(app core.App, messageID string) {
	go func() {
		if err := enqueueNow(app, messageID); err != nil {
			logProgress("enqueue %s: %v", messageID, err)
			return
		}
	}()
}

// Requeue forces a message back onto the analysis queue, clearing any prior
// result. Used by the UI "Re-analyze" action. Synchronous so the HTTP handler
// can report success/failure.
func Requeue(app core.App, messageID string) error {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return fmt.Errorf("messageId required")
	}

	msgCol, err := app.FindCollectionByNameOrId("messages")
	if err != nil {
		return err
	}
	msg, err := app.FindRecordById(msgCol, messageID)
	if err != nil {
		return fmt.Errorf("message not found")
	}

	if excluded, err := folderExcludedForMessage(app, msg); err != nil {
		return err
	} else if excluded {
		return fmt.Errorf("analysis is disabled for this folder")
	}

	if err := forcePending(app, messageID); err != nil {
		return err
	}
	wake()
	return nil
}

func enqueueNow(app core.App, messageID string) error {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return nil
	}

	msgCol, err := app.FindCollectionByNameOrId("messages")
	if err != nil {
		return err
	}
	msg, err := app.FindRecordById(msgCol, messageID)
	if err != nil {
		return fmt.Errorf("message not found: %w", err)
	}

	if excluded, err := folderExcludedForMessage(app, msg); err != nil {
		return err
	} else if excluded {
		return nil
	}

	changed, err := upsertPending(app, messageID)
	if err != nil {
		return err
	}
	if changed {
		wake()
	}
	return nil
}

func folderExcludedForMessage(app core.App, msg *core.Record) (bool, error) {
	folderID := msg.GetString("folder")
	if folderID == "" {
		return false, nil
	}
	folderCol, err := app.FindCollectionByNameOrId("folders")
	if err != nil {
		return false, err
	}
	folder, err := app.FindRecordById(folderCol, folderID)
	if err != nil {
		// Folder missing shouldn't block analysis of an otherwise valid message.
		return false, nil
	}
	return FolderIsExcludedFromAnalysis(folder.GetString("name"), folder.GetString("role")), nil
}

// enqueueMu serializes upsertPending's check-then-insert so two concurrent
// Enqueue calls for the same message can't both observe "no existing row"
// and both insert one. The unique index on message_analysis.message (see
// mailstore) is the hard backstop; this mutex avoids relying on it via a
// Save error in the common case.
var enqueueMu sync.Mutex

// upsertPending creates a pending message_analysis row for messageID, or
// leaves an existing row untouched if one already exists (regardless of its
// status) so we never create duplicate pending rows or disturb an
// in-flight/finished analysis. Returns whether a new row was created.
func upsertPending(app core.App, messageID string) (bool, error) {
	enqueueMu.Lock()
	defer enqueueMu.Unlock()

	col, err := app.FindCollectionByNameOrId("message_analysis")
	if err != nil {
		return false, err
	}
	if _, err := app.FindFirstRecordByFilter(col.Id, "message = {:m}", dbx.Params{"m": messageID}); err == nil {
		return false, nil
	}

	rec := core.NewRecord(col)
	rec.Set("message", messageID)
	rec.Set("status", "pending")
	rec.Set("fail_count", 0)
	if err := app.Save(rec); err != nil {
		return false, err
	}
	return true, nil
}

// forcePending creates or resets the message_analysis row to pending and
// clears prior model output so a fresh completion can replace it.
func forcePending(app core.App, messageID string) error {
	enqueueMu.Lock()
	defer enqueueMu.Unlock()

	col, err := app.FindCollectionByNameOrId("message_analysis")
	if err != nil {
		return err
	}
	rec, err := app.FindFirstRecordByFilter(col.Id, "message = {:m}", dbx.Params{"m": messageID})
	if err != nil {
		rec = core.NewRecord(col)
		rec.Set("message", messageID)
	}
	rec.Set("status", "pending")
	rec.Set("fail_count", 0)
	rec.Set("error", "")
	rec.Set("priority", "")
	rec.Set("suggested_action", "")
	rec.Set("action_target", "")
	rec.Set("suggested_reply", "")
	rec.Set("create_folder", false)
	rec.Set("event_starts_at", "")
	rec.Set("event_ends_at", "")
	rec.Set("event_attendees", "")
	rec.Set("analyzed_at", "")
	return app.Save(rec)
}

// Start runs the crash-recovery/backlog sweep once, then launches the
// single-flight worker goroutine. Safe to call once at server start.
//
// Pending work is selected newest-message-first via a join on messages
// (date DESC, uid DESC); see newestPending.
func Start(app core.App) {
	go func() {
		startupSweep(app)
		runWorker(app)
	}()
}

func startupSweep(app core.App) {
	resetRunningToPending(app)
	sweepMissingAnalysis(app)
}

func resetRunningToPending(app core.App) {
	col, err := app.FindCollectionByNameOrId("message_analysis")
	if err != nil {
		logProgress("sweep: message_analysis collection: %v", err)
		return
	}
	recs, err := app.FindRecordsByFilter(col.Id, "status = 'running'", "", 0, 0)
	if err != nil {
		logProgress("sweep: find running rows: %v", err)
		return
	}
	for _, rec := range recs {
		rec.Set("status", "pending")
		if err := app.Save(rec); err != nil {
			logProgress("sweep: reset running->pending %s: %v", rec.Id, err)
		}
	}
	if len(recs) > 0 {
		logProgress("sweep: reset %d running row(s) to pending", len(recs))
	}
}

type sweepRow struct {
	ID string `db:"id"`
}

// sweepMissingAnalysis enqueues any eligible message (has a body, folder not
// excluded) that has no message_analysis row yet. Processed in batches so we
// never load the whole messages table into memory at once.
func sweepMissingAnalysis(app core.App) {
	total := 0
	for i := 0; i < maxSweepBatches; i++ {
		var rows []sweepRow
		err := app.DB().NewQuery(`
			SELECT m.id AS id
			FROM messages m
			JOIN folders f ON f.id = m.folder
			LEFT JOIN message_analysis a ON a.message = m.id
			WHERE a.id IS NULL
			  AND (COALESCE(m.body_text, '') != '' OR COALESCE(m.body_html, '') != '')
			  AND LOWER(COALESCE(f.role, '')) NOT IN ('trash', 'spam', 'junk')
			  AND LOWER(f.name) NOT LIKE '%trash%'
			  AND LOWER(f.name) NOT LIKE '%deleted%'
			  AND LOWER(f.name) NOT LIKE '%spam%'
			  AND LOWER(f.name) NOT LIKE '%junk%'
			ORDER BY m.date DESC, m.uid DESC
			LIMIT {:limit}
		`).Bind(dbx.Params{"limit": sweepBatchSize}).All(&rows)
		if err != nil {
			logProgress("sweep: query missing analysis: %v", err)
			return
		}
		if len(rows) == 0 {
			break
		}
		progressed := false
		for _, row := range rows {
			created, err := upsertPending(app, row.ID)
			if err != nil {
				logProgress("sweep: create pending for %s: %v", row.ID, err)
				continue
			}
			if created {
				progressed = true
				total++
			}
		}
		if len(rows) < sweepBatchSize {
			break
		}
		if !progressed {
			// Every row in this batch failed to insert; bail out instead of
			// looping forever re-selecting the same rows.
			logProgress("sweep: no progress on batch, stopping early")
			break
		}
	}
	if total > 0 {
		logProgress("sweep: queued %d backlog message(s) for analysis", total)
		wake()
	}
}

// runWorker is the single-flight analysis loop. Exactly one analysis runs at
// a time; it never overlaps with itself since it's a single goroutine.
func runWorker(app core.App) {
	for {
		depth := countPending(app)

		settings, err := LoadSettings(app)
		if err != nil {
			logProgress("load settings: %v", err)
			sleepOrWake(pauseInterval)
			continue
		}
		preferredModel, baseURL := settings.Model, settings.BaseURL

		if !Reachable(baseURL) {
			setStatus(func(s *Status) {
				s.State = "paused"
				s.Message = "waiting for LM Studio at " + baseURL
				s.CurrentMessageID = ""
				s.QueueDepth = depth
			})
			sleepOrWake(pauseInterval)
			continue
		}

		models, err := ListModels(baseURL)
		if err != nil {
			setStatus(func(s *Status) {
				s.State = "paused"
				s.Message = "list models: " + err.Error()
				s.QueueDepth = depth
			})
			sleepOrWake(pauseInterval)
			continue
		}
		resolvedModel, ok := ResolveModel(preferredModel, models)
		if !ok {
			setStatus(func(s *Status) {
				s.State = "paused"
				s.Message = "no usable chat model available at " + baseURL
				s.Model = ""
				s.QueueDepth = depth
			})
			sleepOrWake(pauseInterval)
			continue
		}

		rec, err := newestPending(app)
		if err != nil {
			logProgress("query newest pending: %v", err)
			sleepOrWake(idleInterval)
			continue
		}
		if rec == nil {
			setStatus(func(s *Status) {
				s.State = "idle"
				s.Message = "up to date"
				s.CurrentMessageID = ""
				s.Model = resolvedModel
				s.QueueDepth = 0
			})
			sleepOrWake(idleInterval)
			continue
		}

		processMessage(app, rec, baseURL, resolvedModel, depth)
	}
}

func countPending(app core.App) int {
	col, err := app.FindCollectionByNameOrId("message_analysis")
	if err != nil {
		return 0
	}
	n, err := app.CountRecords(col.Id, dbx.NewExp("status = 'pending'"))
	if err != nil {
		return 0
	}
	return int(n)
}

type pendingRow struct {
	ID string `db:"id"`
}

// newestPending returns the pending analysis row whose linked message is
// newest (messages.date DESC, then uid DESC). Nil when the queue is empty.
func newestPending(app core.App) (*core.Record, error) {
	var rows []pendingRow
	err := app.DB().NewQuery(`
		SELECT a.id AS id
		FROM message_analysis a
		JOIN messages m ON m.id = a.message
		WHERE a.status = 'pending'
		ORDER BY m.date DESC, m.uid DESC
		LIMIT 1
	`).All(&rows)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	col, err := app.FindCollectionByNameOrId("message_analysis")
	if err != nil {
		return nil, err
	}
	return app.FindRecordById(col, rows[0].ID)
}

func processMessage(app core.App, rec *core.Record, baseURL, model string, depth int) {
	messageID := rec.GetString("message")

	rec.Set("status", "running")
	if err := app.Save(rec); err != nil {
		logProgress("mark running for %s: %v", messageID, err)
		return
	}
	setStatus(func(s *Status) {
		s.State = "running"
		s.CurrentMessageID = messageID
		s.Model = model
		s.Message = "analyzing message"
		// The record we just claimed is included in depth; subtract it so
		// queueDepth reflects work still waiting behind the current one.
		if depth > 0 {
			s.QueueDepth = depth - 1
		} else {
			s.QueueDepth = 0
		}
	})

	prompt, session, err := buildUserPrompt(app, messageID)
	if err != nil {
		logProgress("build prompt for %s: %v", messageID, err)
		recordParseFailure(app, rec, err)
		return
	}

	content, err := ChatWithTools(baseURL, model, session, prompt)
	if err != nil {
		// Transport/connectivity failure: put the message back in the queue
		// and let the top of the loop re-enter the pause/poll path.
		logProgress("chat completion for %s: %v", messageID, err)
		rec.Set("status", "pending")
		if saveErr := app.Save(rec); saveErr != nil {
			logProgress("reset pending after transport error for %s: %v", messageID, saveErr)
		}
		setStatus(func(s *Status) {
			s.State = "paused"
			s.Message = "LM Studio error: " + err.Error()
			s.CurrentMessageID = ""
		})
		return
	}

	result, err := ParseResult(content)
	if err != nil {
		recordParseFailure(app, rec, err)
		return
	}

	rec.Set("priority", string(result.Priority))
	rec.Set("suggested_action", string(result.SuggestedAction))
	rec.Set("action_target", result.ActionTarget)
	rec.Set("create_folder", result.CreateFolder)
	rec.Set("suggested_reply", result.SuggestedReply)
	rec.Set("event_starts_at", result.EventStartsAt)
	rec.Set("event_ends_at", result.EventEndsAt)
	rec.Set("event_attendees", calendar.EncodeAttendeesJSON(result.Attendees))
	rec.Set("model", model)
	rec.Set("status", "done")
	rec.Set("error", "")
	rec.Set("fail_count", 0)
	rec.Set("analyzed_at", time.Now().UTC().Format(time.RFC3339))
	if err := app.Save(rec); err != nil {
		logProgress("save result for %s: %v", messageID, err)
		return
	}
	if err := upsertDraftFromAnalysis(app, messageID, result); err != nil {
		logProgress("upsert draft for %s: %v", messageID, err)
	}
}

func recordParseFailure(app core.App, rec *core.Record, cause error) {
	failCount := int(rec.GetFloat("fail_count")) + 1
	rec.Set("fail_count", failCount)
	rec.Set("error", cause.Error())
	if failCount >= maxFailCount {
		rec.Set("status", "skipped")
	} else {
		rec.Set("status", "pending")
	}
	if err := app.Save(rec); err != nil {
		logProgress("save failure for %s: %v", rec.GetString("message"), err)
	}
}

var htmlTagRe = regexp.MustCompile(`(?s)<[^>]*>`)

func stripHTML(s string) string {
	if s == "" {
		return ""
	}
	return strings.TrimSpace(htmlTagRe.ReplaceAllString(s, " "))
}

// clipPromptBody returns the full body unless it exceeds maxPromptBodyChars.
// Oversized bodies keep a large head and tail so late itinerary blocks remain.
func clipPromptBody(body string) string {
	if len(body) <= maxPromptBodyChars {
		return body
	}
	head := promptBodyHeadKeep
	tail := promptBodyTailKeep
	if head+tail >= len(body) {
		return body
	}
	return body[:head] + "\n...[middle truncated]...\n" + body[len(body)-tail:]
}

func buildUserPrompt(app core.App, messageID string) (string, analysisSession, error) {
	msgCol, err := app.FindCollectionByNameOrId("messages")
	if err != nil {
		return "", analysisSession{}, err
	}
	msg, err := app.FindRecordById(msgCol, messageID)
	if err != nil {
		return "", analysisSession{}, fmt.Errorf("message not found: %w", err)
	}
	session := newSession(app, msg)

	folderName := folderNameByID(app, msg.GetString("folder"))
	body := msg.GetString("body_text")
	if strings.TrimSpace(body) == "" {
		body = stripHTML(msg.GetString("body_html"))
	}
	body = clipPromptBody(body)

	var b strings.Builder
	fmt.Fprintf(&b, "Current folder: %s\n", folderName)
	fmt.Fprintf(&b, "From: %s\n", msg.GetString("from_addr"))
	fmt.Fprintf(&b, "To: %s\n", msg.GetString("to_addrs"))
	fmt.Fprintf(&b, "Received for: %s\n", msg.GetString("received_for"))
	fmt.Fprintf(&b, "Date: %s\n", msg.GetString("date"))
	fmt.Fprintf(&b, "Subject: %s\n\n", msg.GetString("subject"))
	b.WriteString(body)
	b.WriteString("\n\nExisting folders (prefer these; create a new folder only if none fit):\n")
	for _, name := range listFolderNames(session) {
		fmt.Fprintf(&b, "- %s\n", name)
	}
	b.WriteString("\nSubjects this user has sent (use get_sent_email_body with the id to read a body):\n")
	sent := listSentSubjects(session)
	if len(sent) == 0 {
		b.WriteString("(none)\n")
	}
	for _, row := range sent {
		fmt.Fprintf(&b, "- id=%s date=%s subject=%s\n", row.ID, row.Date, row.Subject)
	}
	b.WriteString("\nPrevious actions for this sender or receiving address (use get_message_actions for one email):\n")
	actions := listPriorActions(session)
	if len(actions) == 0 {
		b.WriteString("(none recorded)\n")
	}
	for _, row := range actions {
		fmt.Fprintf(&b, "- message=%s action=%s target=%s when=%s\n", row.Message, row.Action, row.Target, row.When)
	}
	return b.String(), session, nil
}
