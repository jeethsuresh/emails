package calendar

import (
	"sort"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

type WindowEvent struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Notes         string `json:"notes"`
	SourceMessage string `json:"sourceMessage"`
	Status        string `json:"status"`
	CalendarID    string `json:"calendarId"`
	CalendarName  string `json:"calendarName"`
	CalendarColor string `json:"calendarColor"`
	AllDay        bool   `json:"allDay"`
	Timezone      string `json:"timezone"`
	StartsAt      string `json:"startsAt"`
	EndsAt        string `json:"endsAt"`
	DisplayStart  string `json:"displayStart"`
	DisplayEnd    string `json:"displayEnd"`
	DisplayDay    string `json:"displayDay"`    // YYYY-MM-DD in display TZ (timed start day / all-day start)
	EditStartWall string `json:"editStartWall"` // wall clock in event timezone (for forms)
	EditEndWall   string `json:"editEndWall"`
	Lane          int      `json:"lane"`
	LaneCount     int      `json:"laneCount"`
	UID           string   `json:"uid"`
	Attendees     []string `json:"attendees"`
}

type WindowResponse struct {
	DisplayTimezone string        `json:"displayTimezone"`
	From            string        `json:"from"`
	To              string        `json:"to"`
	Events          []WindowEvent `json:"events"`
}

type BoundsResponse struct {
	DisplayTimezone string `json:"displayTimezone"`
	View            string `json:"view"`
	Anchor          string `json:"anchor"`
	From            string `json:"from"`
	To              string `json:"to"`
	FromDate        string `json:"fromDate"` // YYYY-MM-DD in display TZ
	ToDate          string `json:"toDate"`   // exclusive YYYY-MM-DD in display TZ
	Days            int    `json:"days,omitempty"`
}

func handleGetWindow(re *core.RequestEvent) error {
	q := re.Request.URL.Query()
	loc, tzName := resolveDisplayLocation(re.App, q.Get("displayTimezone"))
	fromRaw := q.Get("from")
	toRaw := q.Get("to")
	from, err := parseWindowInstant(fromRaw, loc, false)
	if err != nil {
		return re.BadRequestError("from required (RFC3339 or YYYY-MM-DD)", err)
	}
	to, err := parseWindowInstant(toRaw, loc, true)
	if err != nil {
		return re.BadRequestError("to required (RFC3339 or YYYY-MM-DD)", err)
	}
	if !to.After(from) {
		return re.BadRequestError("to must be after from", nil)
	}

	events, err := loadWindowEvents(re.App, from, to, loc)
	if err != nil {
		return re.BadRequestError(err.Error(), err)
	}
	packTimedLanes(events, loc)

	return re.JSON(200, WindowResponse{
		DisplayTimezone: tzName,
		From:            from.Format(time.RFC3339),
		To:              to.Format(time.RFC3339),
		Events:          events,
	})
}

func handleGetBounds(re *core.RequestEvent) error {
	q := re.Request.URL.Query()
	loc, tzName := resolveDisplayLocation(re.App, q.Get("displayTimezone"))
	view := strings.ToLower(strings.TrimSpace(q.Get("view")))
	if view == "" {
		view = "month"
	}
	anchorRaw := strings.TrimSpace(q.Get("anchor"))
	if anchorRaw == "" {
		anchorRaw = time.Now().In(loc).Format("2006-01-02")
	}
	anchor, err := parseWindowInstant(anchorRaw, loc, false)
	if err != nil {
		return re.BadRequestError("invalid anchor", err)
	}
	anchorLocal := anchor.In(loc)
	days := 7
	if d := q.Get("days"); d != "" {
		if n, perr := parsePositiveInt(d); perr == nil && n >= 2 && n <= 7 {
			days = n
		}
	}

	var fromLocal, toLocal time.Time
	switch view {
	case "day":
		y, m, d := anchorLocal.Date()
		fromLocal = time.Date(y, m, d, 0, 0, 0, 0, loc)
		toLocal = fromLocal.AddDate(0, 0, 1)
		days = 1
	case "multi", "multiday", "week":
		// Week starts Monday (v1); shift later if locale week-start lands.
		y, m, d := anchorLocal.Date()
		dayStart := time.Date(y, m, d, 0, 0, 0, 0, loc)
		weekday := int(dayStart.Weekday()) // Sun=0
		mondayOffset := (weekday + 6) % 7
		fromLocal = dayStart.AddDate(0, 0, -mondayOffset)
		toLocal = fromLocal.AddDate(0, 0, days)
	case "month":
		y, m, _ := anchorLocal.Date()
		first := time.Date(y, m, 1, 0, 0, 0, 0, loc)
		// Pad to Monday-start grid covering the month.
		weekday := int(first.Weekday())
		mondayOffset := (weekday + 6) % 7
		fromLocal = first.AddDate(0, 0, -mondayOffset)
		nextMonth := first.AddDate(0, 1, 0)
		toLocal = nextMonth
		for toLocal.Weekday() != time.Monday {
			toLocal = toLocal.AddDate(0, 0, 1)
		}
	case "year":
		y := anchorLocal.Year()
		fromLocal = time.Date(y, 1, 1, 0, 0, 0, 0, loc)
		toLocal = time.Date(y+1, 1, 1, 0, 0, 0, 0, loc)
	case "list":
		// Wide past/future range so list view is a scrollable agenda, not a
		// short timeline window. Past events sit above "today" in the UI.
		y, m, d := anchorLocal.Date()
		dayStart := time.Date(y, m, d, 0, 0, 0, 0, loc)
		fromLocal = dayStart.AddDate(-10, 0, 0)
		toLocal = dayStart.AddDate(10, 0, 0)
	default:
		return re.BadRequestError("unknown view", nil)
	}

	return re.JSON(200, BoundsResponse{
		DisplayTimezone: tzName,
		View:            view,
		Anchor:          anchorLocal.Format("2006-01-02"),
		From:            fromLocal.UTC().Format(time.RFC3339),
		To:              toLocal.UTC().Format(time.RFC3339),
		FromDate:        fromLocal.Format("2006-01-02"),
		ToDate:          toLocal.Format("2006-01-02"),
		Days:            days,
	})
}

func parsePositiveInt(s string) (int, error) {
	n := 0
	for _, r := range strings.TrimSpace(s) {
		if r < '0' || r > '9' {
			return 0, errBadTime
		}
		n = n*10 + int(r-'0')
	}
	if n <= 0 {
		return 0, errBadTime
	}
	return n, nil
}

func loadWindowEvents(app core.App, from, to time.Time, displayLoc *time.Location) ([]WindowEvent, error) {
	calCol, err := app.FindCollectionByNameOrId("calendars")
	if err != nil {
		return nil, err
	}
	evCol, err := app.FindCollectionByNameOrId("events")
	if err != nil {
		return nil, err
	}

	cals, err := app.FindRecordsByFilter(calCol.Id, "id != ''", "name", 0, 0, nil)
	if err != nil {
		return nil, err
	}
	calByID := map[string]*core.Record{}
	visible := map[string]bool{}
	for _, c := range cals {
		calByID[c.Id] = c
		visible[c.Id] = c.GetBool("is_visible")
	}

	// Broad fetch then filter in Go — event count is small for personal calendars.
	rows, err := app.FindRecordsByFilter(evCol.Id, "id != ''", "starts_at", 0, 0, nil)
	if err != nil {
		return nil, err
	}
	wideWindow := to.Sub(from) > 60*24*time.Hour

	out := make([]WindowEvent, 0, len(rows))
	for _, ev := range rows {
		calID := strings.TrimSpace(ev.GetString("calendar"))
		if calID != "" {
			if vis, ok := visible[calID]; ok && !vis {
				continue
			}
		}
		rrule := strings.TrimSpace(ev.GetString("rrule"))
		instances := occurrenceInstants(ev, from, to, displayLoc)
		if len(instances) == 0 {
			if rrule == "" && eventOverlapsWindow(ev, from, to, displayLoc) {
				out = append(out, projectEvent(ev, calByID[calID], displayLoc))
			} else if wideWindow && rrule == "" && strings.TrimSpace(ev.GetString("starts_at")) == "" {
				// Undated events still appear in the wide list agenda.
				out = append(out, projectEvent(ev, calByID[calID], displayLoc))
			}
			continue
		}
		for i, inst := range instances {
			we := projectEventOccurrence(ev, calByID[calID], displayLoc, inst[0], inst[1], i)
			out = append(out, we)
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		di, dj := out[i].Status == "draft", out[j].Status == "draft"
		if di != dj {
			return di
		}
		if out[i].AllDay != out[j].AllDay {
			return out[i].AllDay && !out[j].AllDay
		}
		return out[i].DisplayStart < out[j].DisplayStart
	})
	return out, nil
}

func occurrenceInstants(ev *core.Record, from, to time.Time, displayLoc *time.Location) [][2]time.Time {
	rrule := strings.TrimSpace(ev.GetString("rrule"))
	allDay := ev.GetBool("all_day")
	starts := strings.TrimSpace(ev.GetString("starts_at"))
	ends := strings.TrimSpace(ev.GetString("ends_at"))
	if allDay {
		startDay, ok := parseDateOnly(starts)
		if !ok {
			return nil
		}
		endDay, ok2 := parseDateOnly(ends)
		if !ok2 {
			endDay = startDay.AddDate(0, 0, 1)
		}
		startInst := time.Date(startDay.Year(), startDay.Month(), startDay.Day(), 0, 0, 0, 0, displayLoc).UTC()
		endInst := time.Date(endDay.Year(), endDay.Month(), endDay.Day(), 0, 0, 0, 0, displayLoc).UTC()
		if rrule == "" {
			if startInst.Before(to) && endInst.After(from) {
				return [][2]time.Time{{startInst, endInst}}
			}
			return nil
		}
		return expandRRuleInstances(rrule, true, startInst, endInst, from, to)
	}
	startUTC, err := time.Parse(time.RFC3339, starts)
	if err != nil {
		return nil
	}
	endUTC := startUTC.Add(time.Hour)
	if ends != "" {
		if t, err := time.Parse(time.RFC3339, ends); err == nil {
			endUTC = t
		}
	}
	if rrule == "" {
		if startUTC.Before(to) && endUTC.After(from) {
			return [][2]time.Time{{startUTC, endUTC}}
		}
		return nil
	}
	return expandRRuleInstances(rrule, false, startUTC, endUTC, from, to)
}

func projectEventOccurrence(ev *core.Record, cal *core.Record, displayLoc *time.Location, startUTC, endUTC time.Time, idx int) WindowEvent {
	we := projectEvent(ev, cal, displayLoc)
	if idx > 0 || strings.TrimSpace(ev.GetString("rrule")) != "" {
		we.ID = ev.Id + "#" + startUTC.UTC().Format("20060102T150405")
	}
	if we.AllDay {
		startLocal := startUTC.In(displayLoc)
		endLocal := endUTC.In(displayLoc)
		we.DisplayStart = startLocal.Format("2006-01-02")
		we.DisplayEnd = endLocal.Format("2006-01-02")
		we.DisplayDay = we.DisplayStart
		we.EditStartWall = we.DisplayStart
		if endDay, ok := parseDateOnly(we.DisplayEnd); ok {
			we.EditEndWall = endDay.AddDate(0, 0, -1).Format("2006-01-02")
		}
		return we
	}
	startLocal := startUTC.In(displayLoc)
	endLocal := endUTC.In(displayLoc)
	we.DisplayStart = startLocal.Format("2006-01-02T15:04:05")
	we.DisplayEnd = endLocal.Format("2006-01-02T15:04:05")
	we.DisplayDay = startLocal.Format("2006-01-02")
	eventLoc, _ := loadLocationOrUTC(we.Timezone)
	we.EditStartWall = startUTC.In(eventLoc).Format("2006-01-02T15:04")
	we.EditEndWall = endUTC.In(eventLoc).Format("2006-01-02T15:04")
	we.StartsAt = startUTC.Format(time.RFC3339)
	we.EndsAt = endUTC.Format(time.RFC3339)
	return we
}

func eventOverlapsWindow(ev *core.Record, from, to time.Time, displayLoc *time.Location) bool {
	starts := strings.TrimSpace(ev.GetString("starts_at"))
	ends := strings.TrimSpace(ev.GetString("ends_at"))
	if ev.GetBool("all_day") {
		startDay, ok1 := parseDateOnly(starts)
		endDay, ok2 := parseDateOnly(ends)
		if !ok1 {
			return false
		}
		if !ok2 {
			endDay = startDay.AddDate(0, 0, 1)
		}
		// All-day dates are civil dates; treat as midnight–midnight in display TZ for overlap.
		startInst := time.Date(startDay.Year(), startDay.Month(), startDay.Day(), 0, 0, 0, 0, displayLoc)
		endInst := time.Date(endDay.Year(), endDay.Month(), endDay.Day(), 0, 0, 0, 0, displayLoc)
		return startInst.Before(to) && endInst.After(from)
	}
	startUTC, err1 := time.Parse(time.RFC3339, starts)
	if err1 != nil {
		return false
	}
	endUTC := startUTC.Add(time.Hour)
	if ends != "" {
		if t, err := time.Parse(time.RFC3339, ends); err == nil {
			endUTC = t
		}
	}
	return startUTC.Before(to) && endUTC.After(from)
}

func projectEvent(ev *core.Record, cal *core.Record, displayLoc *time.Location) WindowEvent {
	we := WindowEvent{
		ID:            ev.Id,
		Title:         ev.GetString("title"),
		Notes:         ev.GetString("notes"),
		SourceMessage: ev.GetString("source_message"),
		Status:        ev.GetString("status"),
		AllDay:        ev.GetBool("all_day"),
		Timezone:      strings.TrimSpace(ev.GetString("timezone")),
		StartsAt:      ev.GetString("starts_at"),
		EndsAt:        ev.GetString("ends_at"),
		UID:           ev.GetString("uid"),
		CalendarID:    ev.GetString("calendar"),
		Attendees:     DecodeAttendeesJSON(ev.GetString("attendees")),
	}
	if we.Status == "" {
		we.Status = "approved"
	}
	if we.Timezone == "" {
		we.Timezone = "UTC"
	}
	if cal != nil {
		we.CalendarName = cal.GetString("name")
		we.CalendarColor = cal.GetString("color")
		we.CalendarID = cal.Id
	}
	if we.CalendarColor == "" {
		we.CalendarColor = "#0f6e56"
	}

	if we.AllDay {
		we.DisplayStart = strings.TrimSpace(we.StartsAt)
		we.DisplayEnd = strings.TrimSpace(we.EndsAt)
		we.DisplayDay = we.DisplayStart
		we.EditStartWall = we.DisplayStart
		if endDay, ok := parseDateOnly(we.DisplayEnd); ok {
			// Exclusive → inclusive for the editor.
			we.EditEndWall = endDay.AddDate(0, 0, -1).Format("2006-01-02")
		} else {
			we.EditEndWall = we.DisplayStart
		}
		return we
	}

	startUTC, err := time.Parse(time.RFC3339, strings.TrimSpace(we.StartsAt))
	if err != nil {
		return we
	}
	endUTC := startUTC.Add(time.Hour)
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(we.EndsAt)); err == nil {
		endUTC = t
	}
	startLocal := startUTC.In(displayLoc)
	endLocal := endUTC.In(displayLoc)
	we.DisplayStart = startLocal.Format("2006-01-02T15:04:05")
	we.DisplayEnd = endLocal.Format("2006-01-02T15:04:05")
	we.DisplayDay = startLocal.Format("2006-01-02")

	eventLoc, _ := loadLocationOrUTC(we.Timezone)
	we.EditStartWall = startUTC.In(eventLoc).Format("2006-01-02T15:04")
	we.EditEndWall = endUTC.In(eventLoc).Format("2006-01-02T15:04")
	return we
}

// packTimedLanes assigns overlap lanes per display day using Go display intervals only.
func packTimedLanes(events []WindowEvent, displayLoc *time.Location) {
	byDay := map[string][]int{}
	for i := range events {
		if events[i].AllDay {
			events[i].Lane = 0
			events[i].LaneCount = 1
			continue
		}
		day := events[i].DisplayDay
		if day == "" {
			continue
		}
		byDay[day] = append(byDay[day], i)
	}
	for _, idxs := range byDay {
		type interval struct {
			idx   int
			start time.Time
			end   time.Time
		}
		items := make([]interval, 0, len(idxs))
		for _, idx := range idxs {
			s, e, ok := parseDisplayInterval(events[idx], displayLoc)
			if !ok {
				continue
			}
			items = append(items, interval{idx: idx, start: s, end: e})
		}
		sort.SliceStable(items, func(a, b int) bool {
			if !items[a].start.Equal(items[b].start) {
				return items[a].start.Before(items[b].start)
			}
			return items[a].end.After(items[b].end)
		})
		laneEnds := []time.Time{}
		assigned := make([]int, len(items))
		maxLane := 0
		for i, it := range items {
			lane := -1
			for l, end := range laneEnds {
				if !it.start.Before(end) {
					lane = l
					laneEnds[l] = it.end
					break
				}
			}
			if lane < 0 {
				lane = len(laneEnds)
				laneEnds = append(laneEnds, it.end)
			}
			assigned[i] = lane
			if lane+1 > maxLane {
				maxLane = lane + 1
			}
		}
		if maxLane < 1 {
			maxLane = 1
		}
		for i, it := range items {
			events[it.idx].Lane = assigned[i]
			events[it.idx].LaneCount = maxLane
		}
	}
}

func parseDisplayInterval(ev WindowEvent, loc *time.Location) (time.Time, time.Time, bool) {
	s, err := time.ParseInLocation("2006-01-02T15:04:05", ev.DisplayStart, loc)
	if err != nil {
		return time.Time{}, time.Time{}, false
	}
	e, err := time.ParseInLocation("2006-01-02T15:04:05", ev.DisplayEnd, loc)
	if err != nil {
		e = s.Add(time.Hour)
	}
	if !e.After(s) {
		e = s.Add(time.Minute * 30)
	}
	return s, e, true
}
