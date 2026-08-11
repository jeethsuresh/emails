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
			model, baseURL, err := LoadSettings(re.App)
			if err != nil {
				return re.BadRequestError(err.Error(), err)
			}
			return re.JSON(200, map[string]any{
				"model":   model,
				"baseUrl": baseURL,
			})
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
			if err := SaveSettings(re.App, model, baseURL); err != nil {
				return re.BadRequestError(err.Error(), err)
			}
			savedModel, savedBaseURL, err := LoadSettings(re.App)
			if err != nil {
				return re.BadRequestError(err.Error(), err)
			}
			wake() // let the worker pick up the new settings immediately
			return re.JSON(200, map[string]any{
				"ok":      true,
				"model":   savedModel,
				"baseUrl": savedBaseURL,
			})
		})

		Start(e.App)
		return e.Next()
	})
}
