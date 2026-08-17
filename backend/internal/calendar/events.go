package calendar

import (
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"email.local/backend/internal/mailstore"
)

type eventWriteRequest struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Notes      string `json:"notes"`
	CalendarID string `json:"calendarId"`
	AllDay     bool   `json:"allDay"`
	Timezone   string `json:"timezone"`
	// Wall times in event timezone (or YYYY-MM-DD for all-day). Never browser-local ISO.
	StartWall     string   `json:"startWall"`
	EndWall       string   `json:"endWall"`
	Status        string   `json:"status"`
	SourceMessage string    `json:"sourceMessage"`
	Attendees     *[]string `json:"attendees"`
}

type eventWriteResponse struct {
	OK    bool   `json:"ok"`
	ID    string `json:"id"`
	Event any    `json:"event"`
}

func handleCreateEvent(re *core.RequestEvent) error {
	var req eventWriteRequest
	if err := re.BindBody(&req); err != nil {
		return re.BadRequestError("invalid json", err)
	}
	rec, err := upsertEventRecord(re.App, nil, req)
	if err != nil {
		return re.BadRequestError(err.Error(), err)
	}
	return re.JSON(200, eventWriteResponse{OK: true, ID: rec.Id, Event: eventJSON(rec)})
}

func handleUpdateEvent(re *core.RequestEvent) error {
	id := strings.TrimSpace(re.Request.PathValue("id"))
	if id == "" {
		return re.BadRequestError("id required", nil)
	}
	if i := strings.IndexByte(id, '#'); i >= 0 {
		id = id[:i]
	}
	evCol, err := re.App.FindCollectionByNameOrId("events")
	if err != nil {
		return re.BadRequestError(err.Error(), err)
	}
	rec, err := re.App.FindRecordById(evCol, id)
	if err != nil {
		return re.BadRequestError("event not found", err)
	}
	var req eventWriteRequest
	if err := re.BindBody(&req); err != nil {
		return re.BadRequestError("invalid json", err)
	}
	req.ID = id
	rec, err = upsertEventRecord(re.App, rec, req)
	if err != nil {
		return re.BadRequestError(err.Error(), err)
	}
	return re.JSON(200, eventWriteResponse{OK: true, ID: rec.Id, Event: eventJSON(rec)})
}

func upsertEventRecord(app core.App, existing *core.Record, req eventWriteRequest) (*core.Record, error) {
	evCol, err := app.FindCollectionByNameOrId("events")
	if err != nil {
		return nil, err
	}

	calID := strings.TrimSpace(req.CalendarID)
	tz := strings.TrimSpace(req.Timezone)
	var cal *core.Record
	if calID != "" {
		calCol, err := app.FindCollectionByNameOrId("calendars")
		if err != nil {
			return nil, err
		}
		cal, err = app.FindRecordById(calCol, calID)
		if err != nil {
			return nil, errString("calendar not found")
		}
	} else {
		cal, err = mailstore.FindDefaultCalendar(app)
		if err != nil {
			return nil, err
		}
		calID = cal.Id
	}
	if tz == "" && cal != nil {
		tz = strings.TrimSpace(cal.GetString("timezone"))
	}
	if tz == "" {
		tz = "UTC"
	}
	loc, tz := loadLocationOrUTC(tz)

	var rec *core.Record
	if existing != nil {
		rec = existing
	} else {
		rec = core.NewRecord(evCol)
		rec.Set("uid", NewEventUID())
		rec.Set("created_at", time.Now().UTC().Format(time.RFC3339))
		rec.Set("source_message", strings.TrimSpace(req.SourceMessage))
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		return nil, errString("title required")
	}
	rec.Set("title", title)
	if req.Notes != "" || existing == nil {
		rec.Set("notes", strings.TrimSpace(req.Notes))
	}
	if req.Attendees != nil {
		rec.Set("attendees", EncodeAttendeesJSON(*req.Attendees))
	} else if existing == nil {
		rec.Set("attendees", "")
	}

	status := strings.TrimSpace(req.Status)
	if status == "" {
		if existing != nil {
			status = existing.GetString("status")
		}
		if status == "" {
			status = "approved"
		}
	}
	rec.Set("status", status)
	rec.Set("calendar", calID)
	rec.Set("all_day", req.AllDay)
	rec.Set("timezone", tz)

	if req.AllDay {
		startDay, ok := parseDateOnly(req.StartWall)
		if !ok {
			if t, err := wallToUTC(req.StartWall, tz); err == nil {
				tl := t.In(loc)
				startDay = time.Date(tl.Year(), tl.Month(), tl.Day(), 0, 0, 0, 0, time.UTC)
				ok = true
			}
		} else {
			startDay = time.Date(startDay.Year(), startDay.Month(), startDay.Day(), 0, 0, 0, 0, time.UTC)
		}
		if !ok {
			return nil, errString("start required for all-day (YYYY-MM-DD)")
		}
		endInclusive, okEnd := parseDateOnly(req.EndWall)
		if !okEnd {
			if t, err := wallToUTC(req.EndWall, tz); err == nil {
				tl := t.In(loc)
				endInclusive = time.Date(tl.Year(), tl.Month(), tl.Day(), 0, 0, 0, 0, time.UTC)
				okEnd = true
			}
		} else {
			endInclusive = time.Date(endInclusive.Year(), endInclusive.Month(), endInclusive.Day(), 0, 0, 0, 0, time.UTC)
		}
		if !okEnd {
			return nil, errString("end required for all-day (YYYY-MM-DD)")
		}
		if endInclusive.Before(startDay) {
			return nil, errString("end must be on or after start")
		}
		// Inclusive UI end date → exclusive storage (iCal-style).
		endExclusive := endInclusive.AddDate(0, 0, 1)
		rec.Set("starts_at", startDay.Format("2006-01-02"))
		rec.Set("ends_at", endExclusive.Format("2006-01-02"))
	} else {
		startUTC, err := wallToUTC(req.StartWall, tz)
		if err != nil {
			return nil, errString("start required")
		}
		endUTC, err := wallToUTC(req.EndWall, tz)
		if err != nil {
			return nil, errString("end required")
		}
		if !endUTC.After(startUTC) {
			return nil, errString("end must be after start")
		}
		rec.Set("starts_at", startUTC.Format(time.RFC3339))
		rec.Set("ends_at", endUTC.Format(time.RFC3339))
	}

	if uid := strings.TrimSpace(rec.GetString("uid")); uid == "" {
		rec.Set("uid", NewEventUID())
	}

	if err := app.Save(rec); err != nil {
		return nil, err
	}
	return rec, nil
}

func eventJSON(rec *core.Record) map[string]any {
	return map[string]any{
		"id":             rec.Id,
		"title":          rec.GetString("title"),
		"notes":          rec.GetString("notes"),
		"source_message": rec.GetString("source_message"),
		"created_at":     rec.GetString("created_at"),
		"starts_at":      rec.GetString("starts_at"),
		"ends_at":        rec.GetString("ends_at"),
		"status":         rec.GetString("status"),
		"calendar":       rec.GetString("calendar"),
		"all_day":        rec.GetBool("all_day"),
		"timezone":       rec.GetString("timezone"),
		"uid":            rec.GetString("uid"),
		"attendees":      DecodeAttendeesJSON(rec.GetString("attendees")),
	}
}
