package analyzer

import (
	"github.com/pocketbase/pocketbase/core"
)

// Register wires the analyzer's HTTP routes and starts the background
// worker. Mirrors syncer.Register's OnServe pattern.
func Register(app core.App) {
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		e.Router.GET("/api/email/analyzer/status", func(re *core.RequestEvent) error {
			return re.JSON(200, CurrentStatus())
		})

		e.Router.GET("/api/email/analyzer/settings", func(re *core.RequestEvent) error {
			s, err := LoadSettings(re.App)
			if err != nil {
				return re.BadRequestError(err.Error(), err)
			}
			return re.JSON(200, settingsJSON(s))
		})

		e.Router.POST("/api/email/analyzer/settings", func(re *core.RequestEvent) error {
			var body map[string]any
			if err := re.BindBody(&body); err != nil {
				return re.BadRequestError("invalid json", err)
			}
			model, _ := body["model"].(string)
			baseURL, _ := body["baseUrl"].(string)
			if baseURL == "" {
				baseURL, _ = body["base_url"].(string)
			}
			interval := DefaultSyncIntervalMinutes
			if existing, err := LoadSettings(re.App); err == nil {
				interval = existing.SyncIntervalMinutes
			}
			if v, ok := body["syncIntervalMinutes"]; ok {
				interval = intFromAny(v, DefaultSyncIntervalMinutes)
			} else if v, ok := body["sync_interval_minutes"]; ok {
				interval = intFromAny(v, DefaultSyncIntervalMinutes)
			}
			if err := SaveSettings(re.App, Settings{
				Model:               model,
				BaseURL:             baseURL,
				SyncIntervalMinutes: interval,
			}); err != nil {
				return re.BadRequestError(err.Error(), err)
			}
			saved, err := LoadSettings(re.App)
			if err != nil {
				return re.BadRequestError(err.Error(), err)
			}
			wake() // let the worker pick up the new settings immediately
			out := settingsJSON(saved)
			out["ok"] = true
			return re.JSON(200, out)
		})

		Start(e.App)
		return e.Next()
	})
}

func settingsJSON(s Settings) map[string]any {
	return map[string]any{
		"model":               s.Model,
		"baseUrl":             s.BaseURL,
		"syncIntervalMinutes": s.SyncIntervalMinutes,
	}
}

func intFromAny(v any, def int) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case float32:
		return int(n)
	case int:
		return n
	case int32:
		return int(n)
	case int64:
		return int(n)
	default:
		return def
	}
}
