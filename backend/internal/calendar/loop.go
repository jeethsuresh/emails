package calendar

import (
	"log"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// StartSyncLoop periodically refreshes ICS subscriptions and CalDAV calendars,
// reusing app_settings.sync_interval_minutes (same cadence as mail sync).
func StartSyncLoop(app core.App) {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		var lastRun time.Time
		for range ticker.C {
			mins := loadSyncIntervalMinutes(app)
			if mins < 1 {
				mins = 5
			}
			if !lastRun.IsZero() && time.Since(lastRun) < time.Duration(mins)*time.Minute {
				continue
			}
			lastRun = time.Now()
			res := syncAllRemoteCalendars(app)
			if ok, _ := res["ok"].(bool); !ok {
				log.Printf("calendar sync: %v", res["error"])
			}
		}
	}()
}

func loadSyncIntervalMinutes(app core.App) int {
	col, err := app.FindCollectionByNameOrId("app_settings")
	if err != nil {
		return 5
	}
	rec, err := app.FindFirstRecordByFilter(col.Id, "id != ''", nil)
	if err != nil {
		return 5
	}
	n := int(rec.GetFloat("sync_interval_minutes"))
	if n < 1 {
		return 5
	}
	if n > 60 {
		return 60
	}
	return n
}

// handleCreateLocalCalendar creates a local calendar row.
func handleCreateLocalCalendar(re *core.RequestEvent) error {
	var body map[string]any
	if err := re.BindBody(&body); err != nil {
		return re.BadRequestError("invalid json", err)
	}
	name, _ := body["name"].(string)
	name = strings.TrimSpace(name)
	if name == "" {
		return re.BadRequestError("name required", nil)
	}
	color, _ := body["color"].(string)
	if strings.TrimSpace(color) == "" {
		color = "#0f6e56"
	}
	tz, _ := body["timezone"].(string)
	if strings.TrimSpace(tz) == "" {
		tz = "UTC"
	}
	calCol, err := re.App.FindCollectionByNameOrId("calendars")
	if err != nil {
		return re.BadRequestError(err.Error(), err)
	}
	rec := core.NewRecord(calCol)
	rec.Set("name", name)
	rec.Set("color", color)
	rec.Set("timezone", tz)
	rec.Set("source", "local")
	rec.Set("is_visible", true)
	rec.Set("is_default", false)
	if err := re.App.Save(rec); err != nil {
		return re.BadRequestError(err.Error(), err)
	}
	return re.JSON(200, map[string]any{"ok": true, "id": rec.Id})
}
