package syncer

import (
	"context"
	"fmt"
	"mime"
	"sort"
	"strings"
	"sync"
	"time"

	"email.local/backend/internal/analyzer"
	"email.local/backend/internal/host"
	"email.local/backend/internal/mailmeta"
	"email.local/backend/internal/mailparse"
	"email.local/backend/internal/netbridge"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

const (
	recentBatchSize    = 40 // newest-first catch-up window per folder
	backfillBatchSize  = 25 // older mail paced in background
	bodyFillBatchSize  = 12 // max body-less messages to repair per folder pass
	backfillPause      = 350 * time.Millisecond
	intervalCheckEvery = 5 * time.Second // how often to re-read sync_interval_minutes
)

var (
	mu              sync.Mutex
	catchupRunning  bool
	backfillRunning bool
	syncCancel      context.CancelFunc // cancels in-flight catch-up
	backfillCancel  context.CancelFunc
)

func Register(app core.App) {
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		e.Router.GET("/api/email/sync/status", func(re *core.RequestEvent) error {
			return re.JSON(200, CurrentStatus())
		})
		e.Router.POST("/api/email/sync", func(re *core.RequestEvent) error {
			go Trigger(e.App)
			return re.JSON(200, map[string]any{"ok": true})
		})
		e.Router.POST("/api/email/wipe", func(re *core.RequestEvent) error {
			if err := WipeMail(e.App); err != nil {
				return re.BadRequestError(err.Error(), err)
			}
			go Trigger(e.App)
			return re.JSON(200, map[string]any{"ok": true})
		})
		e.Router.POST("/api/email/messages/{id}/fetch-body", func(re *core.RequestEvent) error {
			id := re.Request.PathValue("id")
			rec, err := FetchMessageBody(e.App, id)
			if err != nil {
				return re.BadRequestError(err.Error(), err)
			}
			return re.JSON(200, map[string]any{
				"ok":        true,
				"id":        rec.Id,
				"body_text": rec.GetString("body_text"),
				"body_html": rec.GetString("body_html"),
				"snippet":   rec.GetString("snippet"),
			})
		})
		e.Router.POST("/api/email/messages/{id}/move", func(re *core.RequestEvent) error {
			id := re.Request.PathValue("id")
			var req moveRequest
			if err := re.BindBody(&req); err != nil {
				return re.BadRequestError("invalid json", err)
			}
			msg, folder, err := MoveMessage(e.App, id, req)
			if err != nil {
				return re.BadRequestError(err.Error(), err)
			}
			return re.JSON(200, map[string]any{
				"ok":       true,
				"id":       msg.Id,
				"folderId": folder.Id,
			})
		})
		e.Router.POST("/api/email/account", func(re *core.RequestEvent) error {
			var m map[string]any
			if err := re.BindBody(&m); err != nil {
				return re.BadRequestError("invalid json", err)
			}
			if err := upsertFromAPI(e.App, m); err != nil {
				return re.BadRequestError(err.Error(), err)
			}
			go Trigger(e.App)
			return re.JSON(200, map[string]any{"ok": true})
		})

		go loop(e.App)
		return e.Next()
	})
}

func upsertFromAPI(app core.App, m map[string]any) error {
	col, err := app.FindCollectionByNameOrId("accounts")
	if err != nil {
		return err
	}
	email, _ := m["email"].(string)
	rec, err := app.FindFirstRecordByFilter(col, "email = {:email}", dbx.Params{"email": email})
	if err != nil {
		rec = core.NewRecord(col)
	}
	rec.Set("email", email)
	rec.Set("username", m["username"])
	rec.Set("password", m["password"])
	rec.Set("imap_host", m["imapHost"])
	rec.Set("imap_port", m["imapPort"])
	imapSec, _ := m["imapSecurity"].(string)
	if imapSec == "" {
		if tls, ok := m["imapTLS"].(bool); ok && !tls {
			imapSec = "none"
		} else {
			imapSec = "tls"
		}
	}
	rec.Set("imap_security", imapSec)
	rec.Set("imap_tls", imapSec == "tls")
	rec.Set("smtp_host", m["smtpHost"])
	rec.Set("smtp_port", m["smtpPort"])
	smtpSec, _ := m["smtpSecurity"].(string)
	if smtpSec == "" {
		if tls, ok := m["smtpTLS"].(bool); ok && !tls {
			smtpSec = "none"
		} else {
			smtpSec = "tls"
		}
	}
	rec.Set("smtp_security", smtpSec)
	rec.Set("smtp_tls", smtpSec == "tls")
	if insecure, ok := m["tlsInsecure"].(bool); ok {
		rec.Set("tls_insecure", insecure)
	}
	return app.Save(rec)
}

func loop(app core.App) {
	interval := syncInterval(app)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	check := time.NewTicker(intervalCheckEvery)
	defer check.Stop()
	for {
		select {
		case <-ticker.C:
			Trigger(app)
		case <-check.C:
			next := syncInterval(app)
			if next != interval {
				ticker.Reset(next)
				interval = next
				logProgress("sync interval updated to %s", next)
			}
		}
	}
}

func syncInterval(app core.App) time.Duration {
	s, err := analyzer.LoadSettings(app)
	if err != nil {
		return time.Duration(analyzer.DefaultSyncIntervalMinutes) * time.Minute
	}
	mins := analyzer.ClampSyncIntervalMinutes(s.SyncIntervalMinutes)
	return time.Duration(mins) * time.Minute
}

// WipeMail stops any in-flight sync, deletes all cached messages, and resets folder sync cursors.
func WipeMail(app core.App) error {
	mu.Lock()
	if syncCancel != nil {
		syncCancel()
		syncCancel = nil
	}
	if backfillCancel != nil {
		backfillCancel()
		backfillCancel = nil
	}
	mu.Unlock()

	setStatus(func(s *Status) {
		s.State = "syncing"
		s.Phase = "start"
		s.Message = "Wiping local mail cache…"
		s.CurrentFolder = ""
		s.FoldersSynced = 0
		s.FoldersTotal = 0
		s.MessagesSynced = 0
	})
	logProgress("stopping sync before wipe")

	deadline := time.Now().Add(45 * time.Second)
	for {
		mu.Lock()
		running := catchupRunning || backfillRunning
		mu.Unlock()
		if !running {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for sync to stop before wipe")
		}
		time.Sleep(50 * time.Millisecond)
	}

	logProgress("wiping local messages + sync cursors")

	if _, err := app.DB().NewQuery("DELETE FROM messages").Execute(); err != nil {
		return fmt.Errorf("delete messages: %w", err)
	}
	if _, err := app.DB().NewQuery(`
		UPDATE folders SET
			sync_uid_max = 0,
			sync_backfill_uid = 0,
			sync_complete = false
	`).Execute(); err != nil {
		return fmt.Errorf("reset folders: %w", err)
	}

	// Second pass in case anything slipped through during teardown.
	if _, err := app.DB().NewQuery("DELETE FROM messages").Execute(); err != nil {
		return fmt.Errorf("delete messages (2): %w", err)
	}

	logProgress("local mail cache wiped")
	return nil
}

// Trigger runs a newest-first catch-up, then continues older mail in a paced background backfill.
func Trigger(app core.App) {
	mu.Lock()
	if catchupRunning {
		mu.Unlock()
		logProgress("sync already running — skipped")
		return
	}
	catchupRunning = true
	if backfillCancel != nil {
		backfillCancel()
		backfillCancel = nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	syncCancel = cancel
	mu.Unlock()

	defer func() {
		cancel()
		mu.Lock()
		catchupRunning = false
		if syncCancel != nil {
			syncCancel = nil
		}
		mu.Unlock()
	}()

	started := time.Now()
	setStatus(func(s *Status) {
		s.State = "syncing"
		s.Phase = "recent"
		s.Message = "Fetching newest mail…"
		s.CurrentFolder = ""
		s.FoldersSynced = 0
		s.FoldersTotal = 0
		s.MessagesSynced = 0
		s.Logs = nil
	})
	logProgress("catch-up started (newest first)")

	needsBackfill, err := syncAll(ctx, app, false)
	if err != nil {
		if ctx.Err() != nil {
			logProgress("catch-up cancelled")
			return
		}
		msg := err.Error()
		state := "error"
		if strings.Contains(msg, "offline") || strings.Contains(msg, "dial") {
			state = "offline"
		}
		logProgress("catch-up failed after %s: %s", time.Since(started).Round(time.Millisecond), msg)
		setStatus(func(s *Status) {
			s.State = state
			s.Phase = "error"
			s.Message = msg
		})
		return
	}
	if ctx.Err() != nil {
		logProgress("catch-up cancelled")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	logProgress("newest mail ready in %s", time.Since(started).Round(time.Millisecond))

	if !needsBackfill {
		setStatus(func(s *Status) {
			s.State = "idle"
			s.Phase = "idle"
			s.Message = "Up to date"
			s.LastSyncAt = &now
			s.CurrentFolder = ""
		})
		return
	}

	setStatus(func(s *Status) {
		s.State = "syncing"
		s.Phase = "backfill"
		s.Message = "Downloading older mail in background…"
		s.LastSyncAt = &now
	})

	bfCtx, bfCancel := context.WithCancel(context.Background())
	mu.Lock()
	backfillCancel = bfCancel
	mu.Unlock()

	go runBackfill(bfCtx, app)
}

func runBackfill(ctx context.Context, app core.App) {
	mu.Lock()
	backfillRunning = true
	mu.Unlock()
	defer func() {
		mu.Lock()
		backfillRunning = false
		if backfillCancel != nil {
			backfillCancel = nil
		}
		mu.Unlock()
	}()

	logProgress("background backfill started")
	for {
		if ctx.Err() != nil {
			logProgress("background backfill cancelled")
			return
		}
		more, err := syncAll(ctx, app, true)
		if err != nil {
			if ctx.Err() != nil {
				logProgress("background backfill cancelled")
				return
			}
			logProgress("backfill error: %v (will retry later)", err)
			setStatus(func(s *Status) {
				s.State = "idle"
				s.Phase = "idle"
				s.Message = "Recent mail ready (backfill paused)"
			})
			return
		}
		if !more {
			now := time.Now().UTC().Format(time.RFC3339)
			logProgress("background backfill complete — full history indexed")
			setStatus(func(s *Status) {
				s.State = "idle"
				s.Phase = "idle"
				s.Message = "Up to date (full history)"
				s.LastSyncAt = &now
				s.CurrentFolder = ""
			})
			return
		}
		select {
		case <-ctx.Done():
			logProgress("background backfill cancelled")
			return
		case <-time.After(backfillPause):
		}
	}
}

// syncAll returns (needsMoreBackfill, error).
func syncAll(ctx context.Context, app core.App, backfillOnly bool) (bool, error) {
	col, err := app.FindCollectionByNameOrId("accounts")
	if err != nil {
		return false, err
	}
	recs, err := app.FindAllRecords(col)
	if err != nil {
		return false, err
	}
	if len(recs) == 0 {
		logProgress("no account configured")
		setStatus(func(s *Status) {
			s.State = "idle"
			s.Phase = "idle"
			s.Message = "No account configured"
		})
		return false, nil
	}

	needsMore := false
	for _, acc := range recs {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		more, err := syncAccount(ctx, app, acc, backfillOnly)
		if err != nil {
			return false, err
		}
		if more {
			needsMore = true
		}
	}
	return needsMore, nil
}

func syncAccount(ctx context.Context, app core.App, acc *core.Record, backfillOnly bool) (bool, error) {
	hostName := acc.GetString("imap_host")
	port := int(acc.GetFloat("imap_port"))
	sec := netbridge.ParseSecurity(acc.GetString("imap_security"), acc.GetBool("imap_tls"))
	insecure := acc.GetBool("tls_insecure")
	addr := fmt.Sprintf("%s:%d", hostName, port)

	phase := "recent"
	if backfillOnly {
		phase = "backfill"
	}
	setStatus(func(s *Status) {
		s.Phase = phase
		s.Message = "Connecting to " + addr
	})
	logProgress("dialing %s (security=%s mode=%s)", addr, sec, phase)

	conn, err := netbridge.Dial("tcp", addr, sec, insecure)
	if err != nil {
		return false, fmt.Errorf("imap dial: %w", err)
	}

	tlsCfg := netbridge.TLSConfig(hostName, insecure)
	var client *imapclient.Client
	switch sec {
	case netbridge.SecuritySTARTTLS:
		logProgress("upgrading connection with STARTTLS")
		client, err = imapclient.NewStartTLS(conn, &imapclient.Options{TLSConfig: tlsCfg})
		if err != nil {
			_ = conn.Close()
			return false, fmt.Errorf("imap starttls: %w", err)
		}
	default:
		client = imapclient.New(conn, &imapclient.Options{TLSConfig: tlsCfg})
	}
	defer client.Close()

	if err := client.Login(acc.GetString("username"), acc.GetString("password")).Wait(); err != nil {
		return false, fmt.Errorf("imap login: %w", err)
	}

	listCmd := client.List("", "*", nil)
	mailboxes, err := listCmd.Collect()
	if err != nil {
		return false, err
	}

	var targets []*imap.ListData
	for _, mb := range mailboxes {
		if skipMailbox(mb) {
			continue
		}
		targets = append(targets, mb)
	}
	sort.SliceStable(targets, func(i, j int) bool {
		return folderPriority(targets[i].Mailbox) < folderPriority(targets[j].Mailbox)
	})

	setStatus(func(s *Status) {
		s.FoldersTotal = len(targets)
		s.FoldersSynced = 0
	})

	needsMore := false
	for i, mbox := range targets {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		setStatus(func(s *Status) {
			s.Phase = phase
			s.CurrentFolder = mbox.Mailbox
			s.FoldersSynced = i
		})
		n, more, err := syncFolder(app, acc, client, mbox.Mailbox, backfillOnly)
		if err != nil {
			logProgress("folder %s failed: %v (continuing)", mbox.Mailbox, err)
			continue
		}
		if more {
			needsMore = true
		}
		setStatus(func(s *Status) {
			s.FoldersSynced = i + 1
			s.MessagesSynced += n
		})
		if n > 0 {
			logProgress("folder %s: +%d messages (%s)", mbox.Mailbox, n, phase)
		}
	}
	return needsMore, nil
}

func skipMailbox(mb *imap.ListData) bool {
	for _, attr := range mb.Attrs {
		if attr == imap.MailboxAttrNoSelect {
			return true
		}
	}
	n := strings.ToLower(mb.Mailbox)
	switch {
	case strings.Contains(n, "all mail"),
		strings.Contains(n, "[gmail]/all"),
		strings.Contains(n, "all_mail"),
		n == "all":
		return true
	default:
		return false
	}
}

func skipBodyFill(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, "spam") ||
		strings.Contains(n, "junk") ||
		strings.Contains(n, "virus")
}

func folderPriority(name string) int {
	switch folderRole(name) {
	case "inbox":
		return 0
	case "sent":
		return 1
	case "drafts":
		return 2
	case "trash":
		return 3
	default:
		return 10
	}
}

// syncFolder fetches newest-first. Catch-up grabs new + a recent window; backfill walks older UIDs.
// Returns (messagesIngested, needsMoreBackfill, error).
func syncFolder(app core.App, acc *core.Record, client *imapclient.Client, name string, backfillOnly bool) (int, bool, error) {
	t0 := time.Now()
	sel, err := client.Select(name, nil).Wait()
	if err != nil {
		return 0, false, err
	}
	folderRec, err := upsertFolder(app, acc.Id, name, sel)
	if err != nil {
		return 0, false, err
	}
	if sel.NumMessages == 0 || sel.UIDNext <= 1 {
		folderRec.Set("sync_complete", true)
		folderRec.Set("sync_backfill_uid", float64(1))
		_ = app.Save(folderRec)
		return 0, false, nil
	}

	uidMax := uint32(folderRec.GetFloat("sync_uid_max"))
	backfillUID := uint32(folderRec.GetFloat("sync_backfill_uid"))
	uidNext := uint32(sel.UIDNext)
	highest := uidNext - 1

	// UIDVALIDITY change → resync from scratch.
	storedValidity := uint32(folderRec.GetFloat("uidvalidity"))
	if storedValidity != 0 && sel.UIDValidity != 0 && storedValidity != uint32(sel.UIDValidity) {
		logProgress("  %s uidvalidity changed — resetting cursor", name)
		uidMax = 0
		backfillUID = 0
		folderRec.Set("sync_complete", false)
	}
	folderRec.Set("uidvalidity", float64(sel.UIDValidity))
	folderRec.Set("uidnext", float64(sel.UIDNext))

	var ranges []uidRange
	if !backfillOnly {
		// 1) Brand-new mail above previous high-water (newest first).
		if uidMax > 0 && highest > uidMax {
			ranges = append(ranges, uidRange{low: uidMax + 1, high: highest})
		}
		// 2) Initial / catch-up recent window (newest first).
		if backfillUID == 0 {
			low := uint32(1)
			if highest > recentBatchSize {
				low = highest - recentBatchSize + 1
			}
			ranges = append(ranges, uidRange{low: low, high: highest})
			backfillUID = low // next older batch ends just below this
		}
	} else {
		if folderRec.GetBool("sync_complete") || backfillUID <= 1 {
			folderRec.Set("sync_complete", true)
			_ = app.Save(folderRec)
			return 0, false, nil
		}
		high := backfillUID - 1
		if high < 1 {
			folderRec.Set("sync_complete", true)
			folderRec.Set("sync_backfill_uid", float64(1))
			_ = app.Save(folderRec)
			return 0, false, nil
		}
		low := uint32(1)
		if high > backfillBatchSize {
			low = high - backfillBatchSize + 1
		}
		ranges = append(ranges, uidRange{low: low, high: high})
		backfillUID = low
	}

	count := 0
	for _, r := range ranges {
		if r.low > r.high {
			continue
		}
		n, err := fetchUIDRangeNewestFirst(app, acc.Id, acc.GetString("email"), folderRec.Id, client, r.low, r.high)
		if err != nil {
			return count, true, err
		}
		count += n
		if uint32(folderRec.GetFloat("sync_uid_max")) < r.high {
			folderRec.Set("sync_uid_max", float64(r.high))
		}
		if uidMax < r.high {
			uidMax = r.high
		}
	}

	// Repair messages that were synced as headers-only before body caching existed.
	if !skipBodyFill(name) {
		if filled, err := fillMissingBodies(app, acc.Id, acc.GetString("email"), folderRec.Id, client, bodyFillBatchSize); err == nil {
			count += filled
		} else {
			logProgress("  %s body fill: %v", name, err)
		}
	}

	folderRec.Set("sync_backfill_uid", float64(backfillUID))
	complete := backfillUID <= 1
	folderRec.Set("sync_complete", complete)
	if err := app.Save(folderRec); err != nil {
		return count, !complete, err
	}

	logProgress("  %s %s: +%d in %s (backfill@%d complete=%v)",
		name, map[bool]string{false: "recent", true: "backfill"}[backfillOnly],
		count, time.Since(t0).Round(time.Millisecond), backfillUID, complete)
	return count, !complete, nil
}

type uidRange struct {
	low, high uint32
}

func fetchUIDSet(app core.App, accountID, accountEmail, folderID string, client *imapclient.Client, uids []uint32) (int, error) {
	if len(uids) == 0 {
		return 0, nil
	}
	count := 0
	// Fetch one UID at a time — Proton Bridge often stalls or returns empty
	// on multi-UID BODY.PEEK[] batches.
	for _, u := range uids {
		bodySection := &imap.FetchItemBodySection{Peek: true}
		set := imap.UIDSet{}
		set.AddNum(imap.UID(u))
		cmd := client.Fetch(set, &imap.FetchOptions{
			Envelope:     true,
			Flags:        true,
			UID:          true,
			InternalDate: true,
			BodySection:  []*imap.FetchItemBodySection{bodySection},
		})
		msg := cmd.Next()
		if msg == nil {
			_ = cmd.Close()
			logProgress("  uid %d: empty FETCH response", u)
			continue
		}
		buf, err := msg.Collect()
		if err != nil || buf == nil {
			_ = cmd.Close()
			logProgress("  uid %d: collect failed: %v", u, err)
			continue
		}
		// Drain any extra messages then close.
		for cmd.Next() != nil {
		}
		if err := cmd.Close(); err != nil {
			logProgress("  uid %d: fetch close: %v", u, err)
			continue
		}
		// Ensure ingest sees body bytes even if section metadata is odd.
		if raw := buf.FindBodySection(bodySection); len(raw) > 0 {
			found := false
			for _, s := range buf.BodySection {
				if len(s.Bytes) > 0 {
					found = true
					break
				}
			}
			if !found {
				buf.BodySection = append(buf.BodySection, imapclient.FetchBodySectionBuffer{
					Section: bodySection,
					Bytes:   raw,
				})
			}
		}
		if err := ingestBuffer(app, accountID, accountEmail, folderID, buf); err != nil {
			logProgress("  uid %d: ingest failed: %v", u, err)
			continue
		}
		count++
	}
	return count, nil
}

func fillMissingBodies(app core.App, accountID, accountEmail, folderID string, client *imapclient.Client, limit int) (int, error) {
	if limit <= 0 {
		return 0, nil
	}
	type row struct {
		UID float64 `db:"uid"`
	}
	var rows []row
	err := app.DB().NewQuery(`
		SELECT uid FROM messages
		WHERE folder = {:folder}
		  AND (body_text IS NULL OR body_text = '')
		ORDER BY date DESC, uid DESC
		LIMIT {:limit}
	`).Bind(dbx.Params{"folder": folderID, "limit": limit}).All(&rows)
	if err != nil {
		logProgress("  body fill query failed: %v", err)
		return 0, err
	}
	uids := make([]uint32, 0, len(rows))
	for _, r := range rows {
		u := uint32(r.UID)
		if u > 0 {
			uids = append(uids, u)
		}
	}
	if len(uids) == 0 {
		return 0, nil
	}
	logProgress("  body fill fetching %d uids", len(uids))
	n, err := fetchUIDSet(app, accountID, accountEmail, folderID, client, uids)
	if err != nil {
		logProgress("  body fill fetch failed (%d uids): %v", len(uids), err)
		return n, err
	}
	if n > 0 {
		logProgress("  filled bodies for %d messages", n)
		msgCol, colErr := app.FindCollectionByNameOrId("messages")
		if colErr == nil {
			for _, u := range uids {
				rec, findErr := app.FindFirstRecordByFilter(msgCol, "folder = {:f} && uid = {:u}", dbx.Params{"f": folderID, "u": float64(u)})
				if findErr != nil {
					continue
				}
				enqueueIfBodied(app, rec)
			}
		}
	}
	return n, nil
}

func fetchUIDRangeNewestFirst(app core.App, accountID, accountEmail, folderID string, client *imapclient.Client, low, high uint32) (int, error) {
	// Walk descending chunks so ingest order is reverse-chronological within the range.
	count := 0
	chunk := uint32(backfillBatchSize)
	for end := high; end >= low; {
		start := low
		if end > low && end-low+1 > chunk {
			start = end - chunk + 1
		}
		uids := make([]uint32, 0, int(end-start+1))
		for u := end; u >= start; u-- {
			uids = append(uids, u)
			if u == start {
				break
			}
		}
		n, err := fetchUIDSet(app, accountID, accountEmail, folderID, client, uids)
		count += n
		if err != nil {
			return count, err
		}

		if start == low {
			break
		}
		end = start - 1
	}
	return count, nil
}

func upsertFolder(app core.App, accountID, name string, sel *imap.SelectData) (*core.Record, error) {
	col, err := app.FindCollectionByNameOrId("folders")
	if err != nil {
		return nil, err
	}
	rec, err := app.FindFirstRecordByFilter(col, "account = {:a} && name = {:n}", dbx.Params{"a": accountID, "n": name})
	if err != nil {
		rec = core.NewRecord(col)
		rec.Set("account", accountID)
		rec.Set("name", name)
		rec.Set("role", folderRole(name))
		rec.Set("sync_uid_max", float64(0))
		rec.Set("sync_backfill_uid", float64(0))
		rec.Set("sync_complete", false)
	}
	if sel != nil {
		rec.Set("uidvalidity", float64(sel.UIDValidity))
		rec.Set("uidnext", float64(sel.UIDNext))
	}
	if err := app.Save(rec); err != nil {
		return nil, err
	}
	return rec, nil
}

func folderRole(name string) string {
	n := strings.ToLower(name)
	switch {
	case n == "inbox" || strings.HasSuffix(n, "/inbox"):
		return "inbox"
	case strings.Contains(n, "sent"):
		return "sent"
	case strings.Contains(n, "draft"):
		return "drafts"
	case strings.Contains(n, "trash") || strings.Contains(n, "deleted"):
		return "trash"
	default:
		return "other"
	}
}

func ingestBuffer(app core.App, accountID, accountEmail, folderID string, buf *imapclient.FetchMessageBuffer) error {
	if buf == nil || buf.UID == 0 {
		return fmt.Errorf("missing uid")
	}
	col, err := app.FindCollectionByNameOrId("messages")
	if err != nil {
		return err
	}

	uid := buf.UID
	subject := ""
	fromAddr := ""
	toAddrs := ""
	date := ""
	messageID := ""
	if buf.Envelope != nil {
		env := buf.Envelope
		subject = env.Subject
		messageID = env.MessageID
		if len(env.From) > 0 {
			fromAddr = formatAddr(env.From[0])
		}
		var tos []string
		for _, a := range env.To {
			tos = append(tos, formatAddr(a))
		}
		toAddrs = strings.Join(tos, ", ")
		if !env.Date.IsZero() {
			date = env.Date.UTC().Format(time.RFC3339)
		}
	}
	if date == "" && !buf.InternalDate.IsZero() {
		date = buf.InternalDate.UTC().Format(time.RFC3339)
	}

	// Prefer raw header Subject / Message-ID when present; also grab full body bytes.
	var rawMessage []byte
	for _, section := range buf.BodySection {
		raw := section.Bytes
		if len(raw) == 0 {
			continue
		}
		// Keep the largest payload — full RFC822 beats header-only.
		if len(raw) > len(rawMessage) {
			rawMessage = raw
		}
		if section.Section != nil &&
			(section.Section.Specifier == imap.PartSpecifierHeader ||
				section.Section.Specifier == imap.PartSpecifierNone) {
			if s := host.MimeHeaderGet(string(raw), "Subject"); s != "" {
				subject = s
			}
			if s := host.MimeHeaderGet(string(raw), "Message-ID"); s != "" {
				messageID = s
			}
			if subject == "" {
				if s := headerLine(raw, "Subject"); s != "" {
					subject = s
				}
			}
			if messageID == "" {
				if s := headerLine(raw, "Message-ID"); s != "" {
					messageID = s
				}
			}
		}
	}

	parsed := mailparse.Parsed{}
	func() {
		defer func() {
			if recover() != nil {
				parsed = mailparse.Parsed{}
			}
		}()
		parsed = mailparse.ParseRFC822(rawMessage)
	}()

	headers := make(map[string]string, 10)
	rawHeaders := string(rawMessage)
	for _, name := range []string{
		"Message-ID",
		"Subject",
		"In-Reply-To",
		"References",
		"Delivered-To",
		"X-Original-To",
		"X-Delivered-To",
		"Envelope-To",
		"To",
		"Cc",
	} {
		headers[name] = host.MimeHeaderGet(rawHeaders, name)
	}

	subject = decodeMIMEWords(subject)
	snippet := parsed.Snippet
	if snippet == "" {
		snippet = subject
	} else {
		snippet = decodeMIMEWords(snippet)
	}

	seen := false
	flagged := false
	for _, f := range buf.Flags {
		if strings.EqualFold(string(f), "\\Seen") {
			seen = true
		}
		if strings.EqualFold(string(f), "\\Flagged") {
			flagged = true
		}
	}

	rec, created := findOrCreateMessage(app, col, accountID, folderID, uid, messageID)
	previousThreadID := strings.TrimSpace(rec.GetString("thread_id"))

	rec.Set("subject", subject)
	rec.Set("from_addr", host.NormalizeContact(fromAddr))
	rec.Set("to_addrs", toAddrs)
	rec.Set("date", date)
	rec.Set("message_id", messageID)
	rec.Set("snippet", snippet)
	if parsed.Text != "" || rec.GetString("body_text") == "" {
		rec.Set("body_text", parsed.Text)
	}
	if parsed.HTML != "" || rec.GetString("body_html") == "" {
		rec.Set("body_html", parsed.HTML)
	}
	rec.Set("seen", seen)
	rec.Set("flagged", flagged)
	searchSrc := subject + " " + fromAddr + " " + toAddrs + " " + snippet
	if parsed.Text != "" {
		// Index a prefix of the body — enough for search, not the whole MIME dump.
		bodyIdx := parsed.Text
		if len(bodyIdx) > 2000 {
			bodyIdx = bodyIdx[:2000]
		}
		searchSrc += " " + bodyIdx
	}
	rec.Set("search_tokens", strings.Join(host.Tokenize(searchSrc), " "))
	rec.Set("content_hash", host.Hash(messageID+"|"+subject+"|"+fmt.Sprintf("%d", len(parsed.Text)+len(parsed.HTML))))
	mailmeta.ApplyMessageMeta(app, rec, headers, accountEmail)
	if err := app.Save(rec); err != nil {
		return fmt.Errorf("save message uid=%d: %w", uid, err)
	}
	if next := strings.TrimSpace(rec.GetString("thread_id")); previousThreadID != "" && previousThreadID != next {
		if err := mailmeta.RecountThread(app, previousThreadID); err != nil {
			logProgress("  uid %d: recount old thread failed: %v", uid, err)
		}
	}
	_ = mailmeta.UpsertThreadFromMessage(app, rec)
	_ = mailmeta.UpsertContactFromMessage(app, rec, created)
	enqueueIfBodied(app, rec)
	return nil
}

// findOrCreateMessage resolves the row a fetched message belongs to. (folder,
// uid) is the natural key, but a message sent from this app is persisted with a
// synthetic uid before the server copy exists; adopting that placeholder by
// Message-ID keeps Sent from showing the same message twice. A genuine copy of
// the same Message-ID in a different folder is left alone.
func findOrCreateMessage(
	app core.App,
	col *core.Collection,
	accountID, folderID string,
	uid imap.UID,
	messageID string,
) (*core.Record, bool) {
	rec, err := app.FindFirstRecordByFilter(col, "folder = {:f} && uid = {:u}", dbx.Params{"f": folderID, "u": float64(uid)})
	if err == nil {
		return rec, false
	}
	if bare := mailmeta.NormalizeMessageID(messageID); bare != "" {
		existing, err := app.FindRecordsByFilter(
			col,
			"account = {:account} && (message_id = {:raw} || message_id = {:bare} || message_id = {:bracketed})",
			"uid",
			0,
			0,
			dbx.Params{
				"account":   accountID,
				"raw":       strings.TrimSpace(messageID),
				"bare":      bare,
				"bracketed": "<" + bare + ">",
			},
		)
		if err == nil {
			for _, candidate := range existing {
				if candidate.GetFloat("uid") > 0 && candidate.GetString("folder") != folderID {
					continue
				}
				candidate.Set("folder", folderID)
				candidate.Set("uid", float64(uid))
				return candidate, false
			}
		}
	}
	rec = core.NewRecord(col)
	rec.Set("folder", folderID)
	rec.Set("account", accountID)
	rec.Set("uid", float64(uid))
	return rec, true
}

func enqueueIfBodied(app core.App, rec *core.Record) {
	if rec == nil {
		return
	}
	if strings.TrimSpace(rec.GetString("body_text")) == "" && strings.TrimSpace(rec.GetString("body_html")) == "" {
		return
	}
	analyzer.Enqueue(app, rec.Id)
}

func decodeMIMEWords(s string) string {
	if s == "" || !strings.Contains(s, "=?") {
		return s
	}
	dec := new(mime.WordDecoder)
	out, err := dec.DecodeHeader(s)
	if err != nil || out == "" {
		return s
	}
	return out
}

func headerLine(raw []byte, name string) string {
	for _, line := range strings.Split(string(raw), "\n") {
		l := strings.TrimRight(line, "\r")
		if len(l) >= len(name)+1 && strings.EqualFold(l[:len(name)+1], name+":") {
			return strings.TrimSpace(l[len(name)+1:])
		}
		// Stop at end of headers.
		if l == "" {
			break
		}
	}
	return ""
}

func formatAddr(a imap.Address) string {
	if a.Mailbox == "" {
		return a.Name
	}
	email := a.Mailbox + "@" + a.Host
	if a.Name != "" {
		return fmt.Sprintf("%s <%s>", a.Name, email)
	}
	return email
}

// FetchMessageBody downloads and caches the RFC822 body for one stored message.
func FetchMessageBody(app core.App, messageID string) (*core.Record, error) {
	msgCol, err := app.FindCollectionByNameOrId("messages")
	if err != nil {
		return nil, err
	}
	rec, err := app.FindRecordById(msgCol, messageID)
	if err != nil {
		return nil, fmt.Errorf("message not found")
	}
	if strings.TrimSpace(rec.GetString("body_text")) != "" {
		return rec, nil
	}

	folderID := rec.GetString("folder")
	accountID := rec.GetString("account")
	uid := uint32(rec.GetFloat("uid"))
	if folderID == "" || accountID == "" || uid == 0 {
		return nil, fmt.Errorf("message missing folder/account/uid")
	}

	folderCol, err := app.FindCollectionByNameOrId("folders")
	if err != nil {
		return nil, err
	}
	folderRec, err := app.FindRecordById(folderCol, folderID)
	if err != nil {
		return nil, fmt.Errorf("folder not found")
	}
	accCol, err := app.FindCollectionByNameOrId("accounts")
	if err != nil {
		return nil, err
	}
	acc, err := app.FindRecordById(accCol, accountID)
	if err != nil {
		return nil, fmt.Errorf("account not found")
	}

	hostName := acc.GetString("imap_host")
	port := int(acc.GetFloat("imap_port"))
	sec := netbridge.ParseSecurity(acc.GetString("imap_security"), acc.GetBool("imap_tls"))
	insecure := acc.GetBool("tls_insecure")
	addr := fmt.Sprintf("%s:%d", hostName, port)

	conn, err := netbridge.Dial("tcp", addr, sec, insecure)
	if err != nil {
		return nil, fmt.Errorf("imap dial: %w", err)
	}
	tlsCfg := netbridge.TLSConfig(hostName, insecure)
	var client *imapclient.Client
	switch sec {
	case netbridge.SecuritySTARTTLS:
		client, err = imapclient.NewStartTLS(conn, &imapclient.Options{TLSConfig: tlsCfg})
		if err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("imap starttls: %w", err)
		}
	default:
		client = imapclient.New(conn, &imapclient.Options{TLSConfig: tlsCfg})
	}
	defer client.Close()

	if err := client.Login(acc.GetString("username"), acc.GetString("password")).Wait(); err != nil {
		return nil, fmt.Errorf("imap login: %w", err)
	}
	if _, err := client.Select(folderRec.GetString("name"), nil).Wait(); err != nil {
		return nil, fmt.Errorf("select: %w", err)
	}
	if _, err := fetchUIDSet(app, accountID, acc.GetString("email"), folderID, client, []uint32{uid}); err != nil {
		return nil, err
	}
	rec, err = app.FindRecordById(msgCol, messageID)
	if err != nil {
		return nil, err
	}
	enqueueIfBodied(app, rec)
	return rec, nil
}
