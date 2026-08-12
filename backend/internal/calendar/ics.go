package calendar

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"email.local/backend/internal/mailstore"

	ics "github.com/arran4/golang-ical"
	"github.com/google/uuid"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

type icsImportRequest struct {
	CalendarID string `json:"calendarId"`
	ICSText    string `json:"icsText"`
	URL        string `json:"url"`
}

func handleICSImport(re *core.RequestEvent) error {
	var req icsImportRequest
	if err := re.BindBody(&req); err != nil {
		return re.BadRequestError("invalid json", err)
	}
	text := strings.TrimSpace(req.ICSText)
	url := strings.TrimSpace(req.URL)
	if text == "" && url == "" {
		return re.BadRequestError("icsText or url required", nil)
	}
	if text == "" {
		body, err := fetchURL(url)
		if err != nil {
			return re.BadRequestError(err.Error(), err)
		}
		text = body
	}

	cal, err := resolveOrCreateICSCalendar(re.App, strings.TrimSpace(req.CalendarID), url)
	if err != nil {
		return re.BadRequestError(err.Error(), err)
	}

	n, err := upsertEventsFromICS(re.App, cal, text)
	if err != nil {
		return re.BadRequestError(err.Error(), err)
	}
	if url != "" && strings.TrimSpace(cal.GetString("ics_url")) == "" {
		cal.Set("ics_url", url)
		_ = re.App.Save(cal)
	}
	return re.JSON(200, map[string]any{
		"ok":         true,
		"calendarId": cal.Id,
		"imported":   n,
	})
}

func handleICSExport(re *core.RequestEvent) error {
	calendarID := strings.TrimSpace(re.Request.URL.Query().Get("calendar"))
	if calendarID == "" {
		return re.BadRequestError("calendar query param required", nil)
	}
	calCol, err := re.App.FindCollectionByNameOrId("calendars")
	if err != nil {
		return re.BadRequestError(err.Error(), err)
	}
	cal, err := re.App.FindRecordById(calCol, calendarID)
	if err != nil {
		return re.BadRequestError("calendar not found", err)
	}
	out, err := exportCalendarICS(re.App, cal)
	if err != nil {
		return re.BadRequestError(err.Error(), err)
	}
	re.Response.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	re.Response.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.ics"`, sanitizeFilename(cal.GetString("name"))))
	return re.String(200, out)
}

func fetchURL(raw string) (string, error) {
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		return "", fmt.Errorf("url must be http(s)")
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(raw)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("fetch ics: HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func resolveOrCreateICSCalendar(app core.App, calendarID, icsURL string) (*core.Record, error) {
	calCol, err := app.FindCollectionByNameOrId("calendars")
	if err != nil {
		return nil, err
	}
	if calendarID != "" {
		return app.FindRecordById(calCol, calendarID)
	}
	rec := core.NewRecord(calCol)
	name := "Imported"
	if icsURL != "" {
		name = "ICS Subscription"
	}
	rec.Set("name", name)
	rec.Set("color", "#5f8f74") // sage
	rec.Set("timezone", "UTC")
	rec.Set("source", "ics")
	rec.Set("is_visible", true)
	rec.Set("is_default", false)
	rec.Set("ics_url", icsURL)
	if err := app.Save(rec); err != nil {
		return nil, err
	}
	return rec, nil
}

func upsertEventsFromICS(app core.App, cal *core.Record, text string) (int, error) {
	parsed, err := ics.ParseCalendar(strings.NewReader(text))
	if err != nil {
		return 0, fmt.Errorf("parse ics: %w", err)
	}
	evCol, err := app.FindCollectionByNameOrId("events")
	if err != nil {
		return 0, err
	}
	calTZ := strings.TrimSpace(cal.GetString("timezone"))
	if calTZ == "" {
		calTZ = "UTC"
	}
	n := 0
	for _, ve := range parsed.Events() {
		uid := propValue(ve, ics.ComponentPropertyUniqueId)
		if uid == "" {
			uid = uuid.NewString()
		}
		title := propValue(ve, ics.ComponentPropertySummary)
		notes := propValue(ve, ics.ComponentPropertyDescription)
		rrule := propValue(ve, ics.ComponentPropertyRrule)
		exdate := joinPropValues(ve, ics.ComponentPropertyExdate)

		allDay, starts, ends, tz := extractEventTimes(ve, calTZ)

		rec, err := findEventByUID(app, evCol, cal.Id, uid)
		if err != nil {
			rec = core.NewRecord(evCol)
			rec.Set("uid", uid)
			rec.Set("calendar", cal.Id)
			rec.Set("source_message", "")
			rec.Set("created_at", time.Now().UTC().Format(time.RFC3339))
			rec.Set("status", "approved")
		}
		rec.Set("title", title)
		rec.Set("notes", notes)
		rec.Set("all_day", allDay)
		rec.Set("timezone", tz)
		rec.Set("starts_at", starts)
		rec.Set("ends_at", ends)
		rec.Set("rrule", rrule)
		rec.Set("exdate", exdate)
		rec.Set("calendar", cal.Id)
		if err := app.Save(rec); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func findEventByUID(app core.App, evCol *core.Collection, calendarID, uid string) (*core.Record, error) {
	return app.FindFirstRecordByFilter(
		evCol.Id,
		"calendar = {:c} && uid = {:u}",
		dbx.Params{"c": calendarID, "u": uid},
	)
}

func exportCalendarICS(app core.App, cal *core.Record) (string, error) {
	evCol, err := app.FindCollectionByNameOrId("events")
	if err != nil {
		return "", err
	}
	events, err := app.FindRecordsByFilter(
		evCol.Id,
		"calendar = {:c}",
		"-starts_at",
		0,
		0,
		dbx.Params{"c": cal.Id},
	)
	if err != nil {
		return "", err
	}

	out := ics.NewCalendar()
	out.SetProductId("-//email.local//Calendar//EN")
	out.SetMethod(ics.MethodPublish)
	name := cal.GetString("name")
	if name != "" {
		out.SetName(name)
	}

	for _, ev := range events {
		uid := strings.TrimSpace(ev.GetString("uid"))
		if uid == "" {
			uid = uuid.NewString()
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
		if exdate := strings.TrimSpace(ev.GetString("exdate")); exdate != "" {
			for _, part := range strings.Split(exdate, ",") {
				part = strings.TrimSpace(part)
				if part != "" {
					ve.AddProperty(ics.ComponentPropertyExdate, part)
				}
			}
		}
		writeEventTimes(ve, ev)
	}
	return out.Serialize(), nil
}

func writeEventTimes(ve *ics.VEvent, ev *core.Record) {
	starts := strings.TrimSpace(ev.GetString("starts_at"))
	ends := strings.TrimSpace(ev.GetString("ends_at"))
	if ev.GetBool("all_day") {
		if t, ok := parseDateOnly(starts); ok {
			ve.SetAllDayStartAt(t)
		}
		if t, ok := parseDateOnly(ends); ok {
			ve.SetAllDayEndAt(t)
		}
		return
	}
	tzName := strings.TrimSpace(ev.GetString("timezone"))
	if tzName == "" {
		tzName = "UTC"
	}
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		loc = time.UTC
		tzName = "UTC"
	}
	if t, err := time.Parse(time.RFC3339, starts); err == nil {
		if tzName != "UTC" {
			ve.SetStartAt(t.In(loc), ics.WithTZID(tzName))
		} else {
			ve.SetStartAt(t.UTC())
		}
	}
	if t, err := time.Parse(time.RFC3339, ends); err == nil {
		if tzName != "UTC" {
			ve.SetEndAt(t.In(loc), ics.WithTZID(tzName))
		} else {
			ve.SetEndAt(t.UTC())
		}
	}
}

func extractEventTimes(ve *ics.VEvent, fallbackTZ string) (allDay bool, starts, ends, tz string) {
	tz = fallbackTZ
	if startProp := ve.GetProperty(ics.ComponentPropertyDtStart); startProp != nil {
		if vals, ok := startProp.ICalParameters["TZID"]; ok && len(vals) > 0 && vals[0] != "" {
			tz = vals[0]
		}
		if vals, ok := startProp.ICalParameters["VALUE"]; ok {
			for _, v := range vals {
				if strings.EqualFold(v, "DATE") {
					allDay = true
				}
			}
		}
	}
	if allDay {
		if t, err := ve.GetAllDayStartAt(); err == nil {
			starts = t.Format("2006-01-02")
		}
		if t, err := ve.GetAllDayEndAt(); err == nil {
			ends = t.Format("2006-01-02")
		} else if starts != "" {
			// iCal all-day often omits DTEND; exclusive end = start+1 day
			if t, ok := parseDateOnly(starts); ok {
				ends = t.AddDate(0, 0, 1).Format("2006-01-02")
			}
		}
		return allDay, starts, ends, tz
	}
	if t, err := ve.GetStartAt(); err == nil {
		starts = t.UTC().Format(time.RFC3339)
	}
	if t, err := ve.GetEndAt(); err == nil {
		ends = t.UTC().Format(time.RFC3339)
	}
	if tz == "" {
		tz = "UTC"
	}
	return false, starts, ends, tz
}

func propValue(ve *ics.VEvent, prop ics.ComponentProperty) string {
	p := ve.GetProperty(prop)
	if p == nil {
		return ""
	}
	return strings.TrimSpace(p.Value)
}

func joinPropValues(ve *ics.VEvent, prop ics.ComponentProperty) string {
	props := ve.GetProperties(prop)
	if len(props) == 0 {
		return ""
	}
	parts := make([]string, 0, len(props))
	for _, p := range props {
		v := strings.TrimSpace(p.Value)
		if v != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, ",")
}

func parseDateOnly(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, true
	}
	if t, err := time.Parse("20060102", s); err == nil {
		return t, true
	}
	return time.Time{}, false
}

func sanitizeFilename(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "calendar"
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "calendar"
	}
	return b.String()
}

// NewEventUID returns a stable iCal-style UID for local creates.
func NewEventUID() string {
	return uuid.NewString()
}

// EnsureEventDefaults fills calendar / all_day / timezone / uid for drafts.
func EnsureEventDefaults(app core.App, fields map[string]any) map[string]any {
	if fields == nil {
		fields = map[string]any{}
	}
	cal, err := mailstore.FindDefaultCalendar(app)
	tz := "UTC"
	calID := ""
	if err == nil && cal != nil {
		calID = cal.Id
		if t := strings.TrimSpace(cal.GetString("timezone")); t != "" {
			tz = t
		}
	}
	fields["calendar"] = calID
	fields["all_day"] = false
	fields["timezone"] = tz
	fields["uid"] = NewEventUID()
	return fields
}
