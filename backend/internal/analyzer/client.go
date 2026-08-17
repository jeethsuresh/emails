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
	analysisSystemPrompt = `You analyze email messages for a personal mail client.

You may call tools to inspect related mail, and list_events_and_todos to avoid suggesting duplicate events or todos (including drafts). Prefer existing folders; do not invent a new folder unless none of the listed folders is a reasonable fit. If you recommend creating a folder, set create_folder to true and put the new folder name in action_target.

When finished, return ONLY a JSON object with these keys:
- "priority": exactly one of "high", "medium", "low"
- "suggested_action": exactly one of "move_to_folder", "move_to_spam", "add_event", "add_todo"
- "action_target": optional string (existing folder name, new folder name, event title, or todo title)
- "create_folder": optional boolean, true only when suggested_action is move_to_folder and the folder does not already exist
- "suggested_reply": optional string draft reply, or null/omit if not appropriate
- "event_starts_at": optional RFC3339 start when suggested_action is add_event and you can determine a start (departure, appointment, check-in, etc.); omit only if truly unknown — do not invent dates
- "event_ends_at": optional RFC3339 end when clearly stated (arrival, meeting end); if you have a start but no end, omit event_ends_at (the client will default to start+1h)
- "attendees": optional array of email addresses for add_event when clearly present

Use add_event for meetings, appointments, flights, travel itineraries, and reservations with a date/time in the email. Extract the best title and any start/end you can find (title + start alone is useful). If a matching event or todo already exists (see list_events_and_todos), do not suggest another. Prefer add_todo for tasks with no calendar date/time.

Do not include markdown fences or any text outside the JSON object.`

	chatTimeout   = 120 * time.Second
	modelsTimeout = 5 * time.Second
	maxToolRounds = 6
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
	Tools       []openaiTool  `json:"tools,omitempty"`
}

type chatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

type toolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content   string     `json:"content"`
			ToolCalls []toolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
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
	return chatCompletion(baseURL, model, []chatMessage{
		{Role: "system", Content: systemOrDefault(system)},
		{Role: "user", Content: user},
	}, nil)
}

func ChatWithTools(baseURL, model string, session analysisSession, userPrompt string) (string, error) {
	messages := []chatMessage{
		{Role: "system", Content: AnalysisSystemPrompt},
		{Role: "user", Content: userPrompt},
	}
	tools := analysisTools()
	for round := 0; round < maxToolRounds; round++ {
		content, calls, err := chatCompletionTools(baseURL, model, messages, tools)
		if err != nil {
			if round == 0 && looksLikeToolsUnsupported(err) {
				return ChatJSON(baseURL, model, AnalysisSystemPrompt, userPrompt)
			}
			return "", err
		}
		if len(calls) == 0 {
			if strings.TrimSpace(content) == "" {
				return "", fmt.Errorf("chat completion: empty assistant content")
			}
			return content, nil
		}
		messages = append(messages, chatMessage{Role: "assistant", Content: content, ToolCalls: calls})
		for _, call := range calls {
			id := call.ID
			if id == "" {
				id = fmt.Sprintf("call_%d", round)
			}
			messages = append(messages, chatMessage{
				Role:       "tool",
				Name:       call.Function.Name,
				ToolCallID: id,
				Content:    runAnalysisTool(session, call.Function.Name, call.Function.Arguments),
			})
		}
	}
	messages = append(messages, chatMessage{
		Role:    "user",
		Content: "Stop calling tools. Return the final JSON object now.",
	})
	return chatCompletion(baseURL, model, messages, nil)
}

func systemOrDefault(system string) string {
	if strings.TrimSpace(system) == "" {
		return AnalysisSystemPrompt
	}
	return system
}

func looksLikeToolsUnsupported(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "tool") && (strings.Contains(msg, "400") || strings.Contains(msg, "unsupported") || strings.Contains(msg, "unknown"))
}

func chatCompletionTools(baseURL, model string, messages []chatMessage, tools []openaiTool) (string, []toolCall, error) {
	parsed, err := postChat(baseURL, model, messages, tools)
	if err != nil {
		return "", nil, err
	}
	msg := parsed.Choices[0].Message
	return strings.TrimSpace(msg.Content), msg.ToolCalls, nil
}

func chatCompletion(baseURL, model string, messages []chatMessage, tools []openaiTool) (string, error) {
	parsed, err := postChat(baseURL, model, messages, tools)
	if err != nil {
		return "", err
	}
	content := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if content == "" {
		return "", fmt.Errorf("chat completion: empty assistant content")
	}
	return content, nil
}

func postChat(baseURL, model string, messages []chatMessage, tools []openaiTool) (*chatResponse, error) {
	url, err := joinURL(baseURL, "/v1/chat/completions")
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(chatRequest{
		Model:       model,
		Messages:    messages,
		Temperature: 0.2,
		Tools:       tools,
	})
	if err != nil {
		return nil, fmt.Errorf("encode chat request: %w", err)
	}

	client := &http.Client{Timeout: chatTimeout}
	resp, err := client.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("chat completion: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read chat response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("chat completion: HTTP %d: %s", resp.StatusCode, trimBody(body))
	}

	var parsed chatResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("decode chat response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("chat completion: empty choices")
	}
	return &parsed, nil
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
