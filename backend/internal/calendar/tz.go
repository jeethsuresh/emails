package calendar

import (
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// resolveDisplayLocation picks the display IANA zone: request override,
// then app_settings.display_timezone, then the process local zone.
func resolveDisplayLocation(app core.App, requested string) (*time.Location, string) {
	name := strings.TrimSpace(requested)
	if name == "" {
		if saved, err := loadDisplayTimezone(app); err == nil {
			name = strings.TrimSpace(saved)
		}
	}
	if name == "" || strings.EqualFold(name, "system") || strings.EqualFold(name, "local") {
		return time.Local, localZoneName()
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.Local, localZoneName()
	}
	return loc, name
}

func localZoneName() string {
	name := time.Local.String()
	if name == "" || name == "Local" {
		return "UTC"
	}
	return name
}

func loadLocationOrUTC(name string) (*time.Location, string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return time.UTC, "UTC"
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC, "UTC"
	}
	return loc, name
}

// parseWindowInstant accepts RFC3339 or YYYY-MM-DD (start of day in loc).
func parseWindowInstant(raw string, loc *time.Location, endOfDay bool) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, errEmptyTime
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.ParseInLocation("2006-01-02", raw, loc); err == nil {
		if endOfDay {
			return t.AddDate(0, 0, 1).UTC(), nil
		}
		return t.UTC(), nil
	}
	if t, err := time.ParseInLocation("2006-01-02T15:04", raw, loc); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.ParseInLocation("2006-01-02T15:04:05", raw, loc); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, errBadTime
}

// wallToUTC interprets a wall clock in eventTZ as a UTC instant.
// wall formats: RFC3339, "2006-01-02T15:04", "2006-01-02T15:04:05", "2006-01-02".
func wallToUTC(wall, eventTZ string) (time.Time, error) {
	wall = strings.TrimSpace(wall)
	if wall == "" {
		return time.Time{}, errEmptyTime
	}
	loc, _ := loadLocationOrUTC(eventTZ)
	if t, err := time.Parse(time.RFC3339, wall); err == nil {
		// If the client already sent an offset, respect it; still store UTC.
		return t.UTC(), nil
	}
	for _, layout := range []string{
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
	} {
		if t, err := time.ParseInLocation(layout, wall, loc); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, errBadTime
}

var (
	errEmptyTime = errString("empty time")
	errBadTime   = errString("invalid time")
)

type errString string

func (e errString) Error() string { return string(e) }
