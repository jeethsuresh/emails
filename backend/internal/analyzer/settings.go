package analyzer

import (
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

const (
	DefaultLLMModel   = "google/gemma-4-e4b"
	DefaultLLMBaseURL = "http://127.0.0.1:1234"
)

func LoadSettings(app core.App) (model, baseURL string, err error) {
	col, err := app.FindCollectionByNameOrId("app_settings")
	if err != nil {
		return "", "", err
	}

	rec, err := app.FindFirstRecordByFilter(col.Id, "id != ''", nil)
	if err != nil {
		rec = core.NewRecord(col)
		rec.Set("llm_model", DefaultLLMModel)
		rec.Set("llm_base_url", DefaultLLMBaseURL)
		if err := app.Save(rec); err != nil {
			return "", "", err
		}
		return DefaultLLMModel, DefaultLLMBaseURL, nil
	}

	model = strings.TrimSpace(rec.GetString("llm_model"))
	baseURL = strings.TrimSpace(rec.GetString("llm_base_url"))
	if model == "" {
		model = DefaultLLMModel
	}
	if baseURL == "" {
		baseURL = DefaultLLMBaseURL
	}
	return model, baseURL, nil
}

func SaveSettings(app core.App, model, baseURL string) error {
	col, err := app.FindCollectionByNameOrId("app_settings")
	if err != nil {
		return err
	}

	model = strings.TrimSpace(model)
	baseURL = strings.TrimSpace(baseURL)
	if model == "" {
		model = DefaultLLMModel
	}
	if baseURL == "" {
		baseURL = DefaultLLMBaseURL
	}

	rec, err := app.FindFirstRecordByFilter(col.Id, "id != ''", nil)
	if err != nil {
		rec = core.NewRecord(col)
	}
	rec.Set("llm_model", model)
	rec.Set("llm_base_url", baseURL)
	return app.Save(rec)
}
