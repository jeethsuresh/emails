package calendar

import (
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

type Settings struct {
	DisplayTimezone string `json:"displayTimezone"`
}

func loadDisplayTimezone(app core.App) (string, error) {
	col, err := app.FindCollectionByNameOrId("app_settings")
	if err != nil {
		return "", err
	}
	rec, err := app.FindFirstRecordByFilter(col.Id, "id != ''", nil)
	if err != nil {
		return "", nil
	}
	return strings.TrimSpace(rec.GetString("display_timezone")), nil
}

func saveDisplayTimezone(app core.App, tz string) error {
	col, err := app.FindCollectionByNameOrId("app_settings")
	if err != nil {
		return err
	}
	rec, err := app.FindFirstRecordByFilter(col.Id, "id != ''", nil)
	if err != nil {
		rec = core.NewRecord(col)
	}
	rec.Set("display_timezone", strings.TrimSpace(tz))
	return app.Save(rec)
}

func handleGetSettings(re *core.RequestEvent) error {
	tz, err := loadDisplayTimezone(re.App)
	if err != nil {
		return re.BadRequestError(err.Error(), err)
	}
	return re.JSON(200, Settings{DisplayTimezone: tz})
}

func handlePostSettings(re *core.RequestEvent) error {
	var body map[string]any
	if err := re.BindBody(&body); err != nil {
		return re.BadRequestError("invalid json", err)
	}
	tz, _ := body["displayTimezone"].(string)
	if tz == "" {
		tz, _ = body["display_timezone"].(string)
	}
	if err := saveDisplayTimezone(re.App, tz); err != nil {
		return re.BadRequestError(err.Error(), err)
	}
	saved, err := loadDisplayTimezone(re.App)
	if err != nil {
		return re.BadRequestError(err.Error(), err)
	}
	return re.JSON(200, map[string]any{
		"ok":              true,
		"displayTimezone": saved,
	})
}
