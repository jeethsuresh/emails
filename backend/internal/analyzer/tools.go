package analyzer

import (
	"encoding/json"
	"fmt"
	"strings"

	"email.local/backend/internal/mailmeta"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

const (
	maxSearchHits     = 20
	maxSentSubjects   = 40
	maxPriorActions   = 20
	maxToolBodyChars  = 6_000
	maxFolderList     = 200
	maxToolResultJSON = 24_000
)

type analysisSession struct {
	App       core.App
	MessageID string
	AccountID string
	From      string
	Received  string
}

type openaiTool struct {
	Type     string             `json:"type"`
	Function openaiToolFunction `json:"function"`
}

type openaiToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

func analysisTools() []openaiTool {
	return []openaiTool{
		{
			Type: "function",
			Function: openaiToolFunction{
				Name:        "get_sent_email_body",
				Description: "Fetch the body of a sent email by id from the sent-subjects list.",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"id":{"type":"string","description":"Sent message id"}},"required":["id"]}`),
			},
		},
		{
			Type: "function",
			Function: openaiToolFunction{
				Name:        "get_message_actions",
				Description: "Fetch actions already taken on a specific email id (moves, todos, events, prior analysis).",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"id":{"type":"string","description":"Message id"}},"required":["id"]}`),
			},
		},
		{
			Type: "function",
			Function: openaiToolFunction{
				Name:        "search_emails",
				Description: "Search mail by keyword. Partial matches are allowed. field is subject, body, sender, receiver, or any.",
				Parameters: json.RawMessage(`{"type":"object","properties":{
					"query":{"type":"string"},
					"field":{"type":"string","enum":["subject","body","sender","receiver","any"]}
				},"required":["query"]}`),
			},
		},
		{
			Type: "function",
			Function: openaiToolFunction{
				Name: "list_events_and_todos",
				Description: "List existing calendar events and todos including drafts, to avoid duplicate suggestions. Optional query filters by title substring.",
				Parameters: json.RawMessage(`{"type":"object","properties":{
					"query":{"type":"string","description":"Optional title substring filter"}
				}}`),
			},
		},
	}
}

func runAnalysisTool(session analysisSession, name, argsJSON string) string {
	var args map[string]any
	if strings.TrimSpace(argsJSON) != "" {
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return toolError("invalid arguments: " + err.Error())
		}
	}
	if args == nil {
		args = map[string]any{}
	}
	var (
		out any
		err error
	)
	switch name {
	case "get_sent_email_body":
		out, err = toolGetSentBody(session, strArg(args, "id"))
	case "get_message_actions":
		out, err = toolGetMessageActions(session, strArg(args, "id"))
	case "search_emails":
		out, err = toolSearchEmails(session, strArg(args, "query"), strArg(args, "field"))
	case "list_events_and_todos":
		out, err = toolListEventsAndTodos(session, strArg(args, "query"))
	default:
		return toolError("unknown tool " + name)
	}
	if err != nil {
		return toolError(err.Error())
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return toolError(err.Error())
	}
	if len(raw) > maxToolResultJSON {
		raw = append(raw[:maxToolResultJSON], []byte("…")...)
	}
	return string(raw)
}

func strArg(args map[string]any, key string) string {
	v, _ := args[key].(string)
	return strings.TrimSpace(v)
}

func toolError(msg string) string {
	raw, _ := json.Marshal(map[string]string{"error": msg})
	return string(raw)
}

type sentSubject struct {
	ID      string `db:"id" json:"id"`
	Date    string `db:"date" json:"date"`
	Subject string `db:"subject" json:"subject"`
}

func listSentSubjects(session analysisSession) []sentSubject {
	var rows []sentSubject
	_ = session.App.DB().NewQuery(`
		SELECT m.id AS id, COALESCE(m.date,'') AS date, COALESCE(m.subject,'') AS subject
		FROM messages m
		JOIN folders f ON f.id = m.folder
		WHERE m.account = {:account}
		  AND LOWER(COALESCE(f.role,'')) = 'sent'
		  AND m.id != {:current}
		ORDER BY m.date DESC, m.uid DESC
		LIMIT {:limit}
	`).Bind(dbx.Params{
		"account": session.AccountID,
		"current": session.MessageID,
		"limit":   maxSentSubjects,
	}).All(&rows)
	return rows
}

func listFolderNames(session analysisSession) []string {
	var rows []struct {
		Name string `db:"name"`
	}
	_ = session.App.DB().NewQuery(`
		SELECT name FROM folders
		WHERE account = {:account}
		ORDER BY name ASC
		LIMIT {:limit}
	`).Bind(dbx.Params{"account": session.AccountID, "limit": maxFolderList}).All(&rows)
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.Name) != "" {
			names = append(names, row.Name)
		}
	}
	return names
}

type priorAction struct {
	Message string `json:"message"`
	Action  string `json:"action"`
	Target  string `json:"target"`
	When    string `json:"when"`
}

func listPriorActions(session analysisSession) []priorAction {
	if _, err := session.App.FindCollectionByNameOrId("mail_actions"); err != nil {
		return nil
	}
	var rows []struct {
		Message string `db:"message"`
		Action  string `db:"action"`
		Target  string `db:"target"`
		When    string `db:"created_at"`
	}
	_ = session.App.DB().NewQuery(`
		SELECT message, action, COALESCE(target,'') AS target, COALESCE(created_at,'') AS created_at
		FROM mail_actions
		WHERE from_addr = {:from} OR received_for = {:recv}
		ORDER BY created_at DESC
		LIMIT {:limit}
	`).Bind(dbx.Params{
		"from":  session.From,
		"recv":  session.Received,
		"limit": maxPriorActions,
	}).All(&rows)
	out := make([]priorAction, 0, len(rows)+maxPriorActions)
	seen := map[string]bool{}
	for _, row := range rows {
		out = append(out, priorAction{Message: row.Message, Action: row.Action, Target: row.Target, When: row.When})
		seen[row.Message+"|"+row.Action+"|"+row.Target] = true
	}

	var filed []struct {
		ID     string `db:"id"`
		Date   string `db:"date"`
		Folder string `db:"folder"`
		Action string `db:"suggested_action"`
		Target string `db:"action_target"`
	}
	_ = session.App.DB().NewQuery(`
		SELECT m.id AS id, COALESCE(m.date,'') AS date, COALESCE(f.name,'') AS folder,
		       COALESCE(a.suggested_action,'') AS suggested_action,
		       COALESCE(a.action_target,'') AS action_target
		FROM messages m
		LEFT JOIN folders f ON f.id = m.folder
		LEFT JOIN message_analysis a ON a.message = m.id AND a.status = 'done'
		WHERE m.account = {:account}
		  AND m.id != {:current}
		  AND (LOWER(m.from_addr) LIKE {:fromlike} OR m.received_for = {:recv})
		ORDER BY m.date DESC
		LIMIT {:limit}
	`).Bind(dbx.Params{
		"account":  session.AccountID,
		"current":  session.MessageID,
		"fromlike": "%" + session.From + "%",
		"recv":     session.Received,
		"limit":    maxPriorActions,
	}).All(&filed)
	for _, row := range filed {
		if row.Folder != "" {
			key := row.ID + "|filed|" + row.Folder
			if !seen[key] {
				out = append(out, priorAction{Message: row.ID, Action: "filed", Target: row.Folder, When: row.Date})
				seen[key] = true
			}
		}
		if row.Action != "" {
			key := row.ID + "|suggested|" + row.Action + "|" + row.Target
			if !seen[key] {
				out = append(out, priorAction{Message: row.ID, Action: "suggested:" + row.Action, Target: row.Target, When: row.Date})
				seen[key] = true
			}
		}
	}
	if len(out) > maxPriorActions*2 {
		out = out[:maxPriorActions*2]
	}
	return out
}

func toolGetSentBody(session analysisSession, id string) (any, error) {
	if id == "" {
		return nil, fmt.Errorf("id required")
	}
	msg, err := session.App.FindRecordById("messages", id)
	if err != nil {
		return nil, fmt.Errorf("message not found")
	}
	if msg.GetString("account") != session.AccountID {
		return nil, fmt.Errorf("message not found")
	}
	folderName := folderNameByID(session.App, msg.GetString("folder"))
	role := folderRoleByID(session.App, msg.GetString("folder"))
	if !strings.EqualFold(role, "sent") {
		return nil, fmt.Errorf("not a sent message")
	}
	body := msg.GetString("body_text")
	if strings.TrimSpace(body) == "" {
		body = stripHTML(msg.GetString("body_html"))
	}
	if len(body) > maxToolBodyChars {
		body = body[:maxToolBodyChars] + "\n...[truncated]"
	}
	return map[string]any{
		"id":      id,
		"folder":  folderName,
		"subject": msg.GetString("subject"),
		"date":    msg.GetString("date"),
		"to":      msg.GetString("to_addrs"),
		"body":    body,
	}, nil
}

func toolGetMessageActions(session analysisSession, id string) (any, error) {
	if id == "" {
		return nil, fmt.Errorf("id required")
	}
	msg, err := session.App.FindRecordById("messages", id)
	if err != nil {
		return nil, fmt.Errorf("message not found")
	}
	if msg.GetString("account") != session.AccountID {
		return nil, fmt.Errorf("message not found")
	}
	var logged []priorAction
	var rows []struct {
		Action string `db:"action"`
		Target string `db:"target"`
		When   string `db:"created_at"`
	}
	_ = session.App.DB().NewQuery(`
		SELECT action, COALESCE(target,'') AS target, COALESCE(created_at,'') AS created_at
		FROM mail_actions WHERE message = {:id} ORDER BY created_at DESC
	`).Bind(dbx.Params{"id": id}).All(&rows)
	for _, row := range rows {
		logged = append(logged, priorAction{Message: id, Action: row.Action, Target: row.Target, When: row.When})
	}

	analysis := map[string]string{}
	if rec, err := session.App.FindFirstRecordByFilter("message_analysis", "message = {:m}", dbx.Params{"m": id}); err == nil {
		analysis["status"] = rec.GetString("status")
		analysis["suggested_action"] = rec.GetString("suggested_action")
		analysis["action_target"] = rec.GetString("action_target")
		analysis["priority"] = rec.GetString("priority")
	}

	var extras []map[string]string
	for _, col := range []string{"todos", "events"} {
		recs, err := session.App.FindRecordsByFilter(col, "source_message = {:m}", "", 20, 0, dbx.Params{"m": id})
		if err != nil {
			continue
		}
		for _, rec := range recs {
			extras = append(extras, map[string]string{
				"kind":   col,
				"title":  rec.GetString("title"),
				"status": rec.GetString("status"),
			})
		}
	}

	return map[string]any{
		"id":           id,
		"subject":      msg.GetString("subject"),
		"folder":       folderNameByID(session.App, msg.GetString("folder")),
		"from":         msg.GetString("from_addr"),
		"received_for": msg.GetString("received_for"),
		"actions":      logged,
		"analysis":     analysis,
		"items":        extras,
	}, nil
}

type searchHit struct {
	ID      string `db:"id" json:"id"`
	Date    string `db:"date" json:"date"`
	Subject string `db:"subject" json:"subject"`
	From    string `db:"from_addr" json:"from"`
	To      string `db:"received_for" json:"received_for"`
	Folder  string `db:"folder" json:"folder"`
}

func toolSearchEmails(session analysisSession, query, field string) (any, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("query required")
	}
	like := "%" + escapeLike(query) + "%"
	clause := "1 = 0"
	switch strings.ToLower(field) {
	case "subject":
		clause = "m.subject LIKE {:q} ESCAPE '\\'"
	case "body":
		clause = "(m.body_text LIKE {:q} ESCAPE '\\' OR m.snippet LIKE {:q} ESCAPE '\\')"
	case "sender":
		clause = "m.from_addr LIKE {:q} ESCAPE '\\'"
	case "receiver":
		clause = "(m.to_addrs LIKE {:q} ESCAPE '\\' OR m.received_for LIKE {:q} ESCAPE '\\')"
	default:
		clause = `(m.subject LIKE {:q} ESCAPE '\' OR m.from_addr LIKE {:q} ESCAPE '\'
			OR m.to_addrs LIKE {:q} ESCAPE '\' OR m.received_for LIKE {:q} ESCAPE '\'
			OR m.body_text LIKE {:q} ESCAPE '\' OR m.snippet LIKE {:q} ESCAPE '\'
			OR m.search_tokens LIKE {:q} ESCAPE '\')`
	}
	var rows []searchHit
	err := session.App.DB().NewQuery(`
		SELECT m.id AS id, COALESCE(m.date,'') AS date, COALESCE(m.subject,'') AS subject,
		       COALESCE(m.from_addr,'') AS from_addr, COALESCE(m.received_for,'') AS received_for,
		       COALESCE(f.name,'') AS folder
		FROM messages m
		LEFT JOIN folders f ON f.id = m.folder
		WHERE m.account = {:account} AND m.id != {:current} AND ` + clause + `
		ORDER BY m.date DESC
		LIMIT {:limit}
	`).Bind(dbx.Params{
		"account": session.AccountID,
		"current": session.MessageID,
		"q":       like,
		"limit":   maxSearchHits,
	}).All(&rows)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []searchHit{}
	}
	return map[string]any{"query": query, "field": fieldOrAny(field), "items": rows}, nil
}

const maxEventsTodosHits = 40

func toolListEventsAndTodos(session analysisSession, query string) (any, error) {
	query = strings.ToLower(strings.TrimSpace(query))
	type item struct {
		Kind      string `json:"kind"`
		ID        string `json:"id"`
		Title     string `json:"title"`
		Status    string `json:"status"`
		StartsAt  string `json:"starts_at,omitempty"`
		EndsAt    string `json:"ends_at,omitempty"`
		Deadline  string `json:"deadline,omitempty"`
		SourceMsg string `json:"source_message,omitempty"`
	}
	out := make([]item, 0, maxEventsTodosHits)

	appendFiltered := func(kind, id, title, status, starts, ends, deadline, source string) {
		if len(out) >= maxEventsTodosHits {
			return
		}
		title = strings.TrimSpace(title)
		if query != "" && !strings.Contains(strings.ToLower(title), query) {
			return
		}
		out = append(out, item{
			Kind:      kind,
			ID:        id,
			Title:     title,
			Status:    status,
			StartsAt:  starts,
			EndsAt:    ends,
			Deadline:  deadline,
			SourceMsg: source,
		})
	}

	if evCol, err := session.App.FindCollectionByNameOrId("events"); err == nil {
		rows, err := session.App.FindRecordsByFilter(evCol.Id, "id != ''", "-created_at", maxEventsTodosHits, 0, nil)
		if err == nil {
			for _, rec := range rows {
				appendFiltered(
					"event",
					rec.Id,
					rec.GetString("title"),
					rec.GetString("status"),
					rec.GetString("starts_at"),
					rec.GetString("ends_at"),
					"",
					rec.GetString("source_message"),
				)
			}
		}
	}
	if todoCol, err := session.App.FindCollectionByNameOrId("todos"); err == nil {
		rows, err := session.App.FindRecordsByFilter(todoCol.Id, "id != ''", "-created_at", maxEventsTodosHits, 0, nil)
		if err == nil {
			for _, rec := range rows {
				appendFiltered(
					"todo",
					rec.Id,
					rec.GetString("title"),
					rec.GetString("status"),
					"",
					"",
					rec.GetString("deadline"),
					rec.GetString("source_message"),
				)
			}
		}
	}
	if len(out) > maxEventsTodosHits {
		out = out[:maxEventsTodosHits]
	}
	return map[string]any{"query": query, "items": out}, nil
}

func fieldOrAny(field string) string {
	switch strings.ToLower(strings.TrimSpace(field)) {
	case "subject", "body", "sender", "receiver":
		return strings.ToLower(field)
	default:
		return "any"
	}
}

func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

func folderNameByID(app core.App, id string) string {
	if id == "" {
		return ""
	}
	folder, err := app.FindRecordById("folders", id)
	if err != nil {
		return ""
	}
	return folder.GetString("name")
}

func folderRoleByID(app core.App, id string) string {
	if id == "" {
		return ""
	}
	folder, err := app.FindRecordById("folders", id)
	if err != nil {
		return ""
	}
	return folder.GetString("role")
}

func newSession(app core.App, msg *core.Record) analysisSession {
	return analysisSession{
		App:       app,
		MessageID: msg.Id,
		AccountID: msg.GetString("account"),
		From:      mailmeta.NormalizeEmail(msg.GetString("from_addr")),
		Received:  mailmeta.NormalizeEmail(msg.GetString("received_for")),
	}
}
