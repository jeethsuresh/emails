package analyzer

import (
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

const (
	DefaultLLMModel            = "google/gemma-4-e4b"
	DefaultLLMBaseURL          = "http://127.0.0.1:1234"
	DefaultSyncIntervalMinutes = 5
	MinSyncIntervalMinutes     = 1
	MaxSyncIntervalMinutes     = 60
)

// Settings is the combined analyzer + sync app settings payload.
type Settings struct {
	Model               string
	BaseURL             string
	SyncIntervalMinutes int
}

func ClampSyncIntervalMinutes(n int) int {
	if n < MinSyncIntervalMinutes {
		return DefaultSyncIntervalMinutes
	}
	if n > MaxSyncIntervalMinutes {
		return MaxSyncIntervalMinutes
	}
	return n
}

func syncIntervalFromRecord(rec *core.Record) int {
	n := int(rec.GetFloat("sync_interval_minutes"))
	if n < MinSyncIntervalMinutes || n > MaxSyncIntervalMinutes {
		return DefaultSyncIntervalMinutes
	}
	return n
}

func LoadSettings(app core.App) (Settings, error) {
	col, err := app.FindCollectionByNameOrId("app_settings")
	if err != nil {
		return Settings{}, err
	}

	rec, err := app.FindFirstRecordByFilter(col.Id, "id != ''", nil)
	if err != nil {
		rec = core.NewRecord(col)
		rec.Set("llm_model", DefaultLLMModel)
		rec.Set("llm_base_url", DefaultLLMBaseURL)
		rec.Set("sync_interval_minutes", float64(DefaultSyncIntervalMinutes))
		if err := app.Save(rec); err != nil {
			return Settings{}, err
		}
		return Settings{
			Model:               DefaultLLMModel,
			BaseURL:             DefaultLLMBaseURL,
			SyncIntervalMinutes: DefaultSyncIntervalMinutes,
		}, nil
	}

	model := strings.TrimSpace(rec.GetString("llm_model"))
	baseURL := strings.TrimSpace(rec.GetString("llm_base_url"))
	if model == "" {
		model = DefaultLLMModel
	}
	if baseURL == "" {
		baseURL = DefaultLLMBaseURL
	}
	return Settings{
		Model:               model,
		BaseURL:             baseURL,
		SyncIntervalMinutes: syncIntervalFromRecord(rec),
	}, nil
}

func SaveSettings(app core.App, s Settings) error {
	col, err := app.FindCollectionByNameOrId("app_settings")
	if err != nil {
		return err
	}

	model := strings.TrimSpace(s.Model)
	baseURL := strings.TrimSpace(s.BaseURL)
	if model == "" {
		model = DefaultLLMModel
	}
	if baseURL == "" {
		baseURL = DefaultLLMBaseURL
	}
	interval := ClampSyncIntervalMinutes(s.SyncIntervalMinutes)

	rec, err := app.FindFirstRecordByFilter(col.Id, "id != ''", nil)
	if err != nil {
		rec = core.NewRecord(col)
	}
	rec.Set("llm_model", model)
	rec.Set("llm_base_url", baseURL)
	rec.Set("sync_interval_minutes", float64(interval))
	return app.Save(rec)
}
