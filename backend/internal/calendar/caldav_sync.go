package calendar

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	ics "github.com/arran4/golang-ical"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

type caldavSubscribeRequest struct {
	URL          string `json:"url"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	CalendarPath string `json:"calendarPath"`
	DisplayName  string `json:"displayName"`
	Color        string `json:"color"`
	Timezone     string `json:"timezone"`
}

func handleCalDAVSubscribe(re *core.RequestEvent) error {
	var req caldavSubscribeRequest
	if err := re.BindBody(&req); err != nil {
		return re.BadRequestError("invalid json", err)
	}
	base := strings.TrimSpace(req.URL)
	user := strings.TrimSpace(req.Username)
	path := strings.TrimSpace(req.CalendarPath)
	if base == "" || user == "" || path == "" {
		return re.BadRequestError("url, username, and calendarPath required", nil)
	}
	calCol, err := re.App.FindCollectionByNameOrId("calendars")
	if err != nil {
		return re.BadRequestError(err.Error(), err)
	}
	name := strings.TrimSpace(req.DisplayName)
	if name == "" {
		name = "CalDAV"
	}
	color := strings.TrimSpace(req.Color)
	if color == "" {
		color = "#3d4f5f" // ink
	}
	tz := strings.TrimSpace(req.Timezone)
	if tz == "" {
		tz = "UTC"
	}
	rec := core.NewRecord(calCol)
	rec.Set("name", name)
	rec.Set("color", color)
	rec.Set("timezone", tz)
	rec.Set("source", "caldav")
	rec.Set("is_visible", true)
	rec.Set("is_default", false)
	rec.Set("caldav_url", base)
	rec.Set("caldav_username", user)
	rec.Set("caldav_secret", req.Password)
	rec.Set("caldav_calendar_path", path)
	if err := re.App.Save(rec); err != nil {
		return re.BadRequestError(err.Error(), err)
	}
	n, syncErr := syncCalDAVCalendar(re.App, rec)
	if syncErr != nil {
		rec.Set("last_error", syncErr.Error())
		_ = re.App.Save(rec)
	} else {
		rec.Set("last_error", "")
		rec.Set("last_sync_at", time.Now().UTC().Format(time.RFC3339))
		_ = re.App.Save(rec)
	}
	return re.JSON(200, map[string]any{
		"ok":         syncErr == nil,
		"calendarId": rec.Id,
		"imported":   n,
		"error":      errStringOrEmpty(syncErr),
	})
}

func errStringOrEmpty(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func syncCalDAVCalendar(app core.App, cal *core.Record) (int, error) {
	base := strings.TrimSpace(cal.GetString("caldav_url"))
	path := strings.TrimSpace(cal.GetString("caldav_calendar_path"))
	user := strings.TrimSpace(cal.GetString("caldav_username"))
	pass := cal.GetString("caldav_secret")
	if base == "" || path == "" || user == "" {
		return 0, fmt.Errorf("caldav calendar missing url/path/username")
	}
	href := resolveRef(base, path)
	body, err := reportCalendarData(href, user, pass)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, chunk := range extractICSChunks(body) {
		c, err := upsertEventsFromICS(app, cal, chunk)
		if err != nil {
			return n, err
		}
		n += c
	}
	pushed, err := pushCalDAVEvents(app, cal, href, user, pass)
	if err != nil {
		return n, fmt.Errorf("pull ok (%d) but push failed: %w", n, err)
	}
	_ = pushed
	return n, nil
}

func pushCalDAVEvents(app core.App, cal *core.Record, calendarHref, user, pass string) (int, error) {
	evCol, err := app.FindCollectionByNameOrId("events")
	if err != nil {
		return 0, err
	}
	rows, err := app.FindRecordsByFilter(
		evCol.Id,
		"calendar = {:c} && status = {:s}",
		"",
		0,
		0,
		dbx.Params{"c": cal.Id, "s": "approved"},
	)
	if err != nil {
		return 0, err
	}
	client := &http.Client{Timeout: 30 * time.Second}
	n := 0
	for _, ev := range rows {
		uid := strings.TrimSpace(ev.GetString("uid"))
		if uid == "" {
			uid = NewEventUID()
			ev.Set("uid", uid)
			_ = app.Save(ev)
		}
		payload, err := serializeEventICS(ev)
		if err != nil {
			return n, err
		}
		obj := strings.TrimRight(calendarHref, "/") + "/" + sanitizeFilename(uid) + ".ics"
		req, err := http.NewRequest(http.MethodPut, obj, strings.NewReader(payload))
		if err != nil {
			return n, err
		}
		req.SetBasicAuth(user, pass)
		req.Header.Set("Content-Type", "text/calendar; charset=utf-8")
		if etag := strings.TrimSpace(ev.GetString("etag")); etag != "" {
			req.Header.Set("If-Match", etag)
		}
		resp, err := client.Do(req)
		if err != nil {
			return n, err
		}
		etag := resp.Header.Get("ETag")
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusPreconditionFailed {
			// Remote changed; leave for next pull.
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return n, fmt.Errorf("PUT %s: HTTP %d", obj, resp.StatusCode)
		}
		if etag != "" {
			ev.Set("etag", etag)
			_ = app.Save(ev)
		}
		n++
	}
	return n, nil
}

func serializeEventICS(ev *core.Record) (string, error) {
	out := ics.NewCalendar()
	out.SetProductId("-//email.local//Calendar//EN")
	out.SetMethod(ics.MethodPublish)
	uid := strings.TrimSpace(ev.GetString("uid"))
	if uid == "" {
		uid = NewEventUID()
	}
	ve := out.AddEvent(uid)
	if title := ev.GetString("title"); title != "" {
		ve.SetSummary(title)
	}
	if notes := ev.GetString("notes"); notes != "" {
		ve.SetDescription(notes)
	}
	if rrule := strings.TrimSpace(ev.GetString("rrule")); rrule != "" {
		ve.AddProperty(ics.ComponentPropertyRrule, rrule)
	}
	writeEventTimes(ve, ev)
	return out.Serialize(), nil
}

func refreshICSSubscriptions(app core.App) (int, error) {
	calCol, err := app.FindCollectionByNameOrId("calendars")
	if err != nil {
		return 0, err
	}
	rows, err := app.FindRecordsByFilter(calCol.Id, "source = {:s}", "", 0, 0, dbx.Params{"s": "ics"})
	if err != nil {
		return 0, err
	}
	total := 0
	for _, cal := range rows {
		url := strings.TrimSpace(cal.GetString("ics_url"))
		if url == "" {
			continue
		}
		text, err := fetchURL(url)
		if err != nil {
			cal.Set("last_error", err.Error())
			_ = app.Save(cal)
			continue
		}
		n, err := upsertEventsFromICS(app, cal, text)
		if err != nil {
			cal.Set("last_error", err.Error())
			_ = app.Save(cal)
			continue
		}
		cal.Set("last_error", "")
		cal.Set("last_sync_at", time.Now().UTC().Format(time.RFC3339))
		_ = app.Save(cal)
		total += n
	}
	return total, nil
}

func syncAllRemoteCalendars(app core.App) map[string]any {
	calCol, err := app.FindCollectionByNameOrId("calendars")
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	icsN, _ := refreshICSSubscriptions(app)
	rows, err := app.FindRecordsByFilter(calCol.Id, "source = {:s}", "", 0, 0, dbx.Params{"s": "caldav"})
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error(), "icsImported": icsN}
	}
	results := make([]map[string]any, 0, len(rows))
	for _, cal := range rows {
		n, err := syncCalDAVCalendar(app, cal)
		item := map[string]any{"calendarId": cal.Id, "imported": n}
		if err != nil {
			cal.Set("last_error", err.Error())
			_ = app.Save(cal)
			item["error"] = err.Error()
		} else {
			cal.Set("last_error", "")
			cal.Set("last_sync_at", time.Now().UTC().Format(time.RFC3339))
			_ = app.Save(cal)
			item["ok"] = true
		}
		results = append(results, item)
	}
	return map[string]any{"ok": true, "icsImported": icsN, "results": results}
}

func handleCalendarSyncAll(re *core.RequestEvent) error {
	return re.JSON(200, syncAllRemoteCalendars(re.App))
}

func handleICSRefresh(re *core.RequestEvent) error {
	n, err := refreshICSSubscriptions(re.App)
	if err != nil {
		return re.BadRequestError(err.Error(), err)
	}
	return re.JSON(200, map[string]any{"ok": true, "imported": n})
}
