package analyzer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	analysisSystemPrompt = `You analyze email messages. Return ONLY a JSON object with these keys:
- "priority": exactly one of "high", "medium", "low"
- "suggested_action": exactly one of "move_to_folder", "move_to_spam", "add_event", "add_todo"
- "action_target": optional string (folder name, event title, todo title, etc.)
- "suggested_reply": optional string draft reply, or null/omit if not appropriate

Do not include markdown fences or any text outside the JSON object.`

	chatTimeout   = 60 * time.Second
	modelsTimeout = 5 * time.Second
)

// AnalysisSystemPrompt is the default system prompt for email analysis completions.
const AnalysisSystemPrompt = analysisSystemPrompt

type modelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func ListModels(baseURL string) ([]ModelInfo, error) {
	url, err := joinURL(baseURL, "/v1/models")
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: modelsTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read models response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list models: HTTP %d: %s", resp.StatusCode, trimBody(body))
	}

	var parsed modelsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("decode models response: %w", err)
	}

	out := make([]ModelInfo, 0, len(parsed.Data))
	for _, item := range parsed.Data {
		if item.ID == "" {
			continue
		}
		out = append(out, ModelInfo{ID: item.ID})
	}
	return out, nil
}

func ChatJSON(baseURL, model, system, user string) (string, error) {
	url, err := joinURL(baseURL, "/v1/chat/completions")
	if err != nil {
		return "", err
	}

	if strings.TrimSpace(system) == "" {
		system = AnalysisSystemPrompt
	}

	payload, err := json.Marshal(chatRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temperature: 0.2,
	})
	if err != nil {
		return "", fmt.Errorf("encode chat request: %w", err)
	}

	client := &http.Client{Timeout: chatTimeout}
	resp, err := client.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("chat completion: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read chat response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("chat completion: HTTP %d: %s", resp.StatusCode, trimBody(body))
	}

	var parsed chatResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("decode chat response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("chat completion: empty choices")
	}

	content := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if content == "" {
		return "", fmt.Errorf("chat completion: empty assistant content")
	}
	return content, nil
}

func Reachable(baseURL string) bool {
	_, err := ListModels(baseURL)
	return err == nil
}

func joinURL(baseURL, path string) (string, error) {
	base := strings.TrimSpace(baseURL)
	if base == "" {
		return "", fmt.Errorf("base URL required")
	}
	return strings.TrimRight(base, "/") + path, nil
}

func trimBody(body []byte) string {
	s := strings.TrimSpace(string(body))
	if len(s) > 512 {
		return s[:512] + "..."
	}
	return s
}
