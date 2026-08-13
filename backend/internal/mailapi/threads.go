package mailapi

import (
	"math"
	"strconv"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// maxThreadMessages bounds a single thread payload so a runaway thread cannot
// pull unlimited full message bodies into memory.
const maxThreadMessages = 500

type idRow struct {
	ID string `db:"id"`
}

type countRow struct {
	Total int `db:"total"`
}

func handleListThreads(re *core.RequestEvent) error {
	query := re.Request.URL.Query()
	page, perPage := pagination(query.Get("page"), query.Get("perPage"))
	ids, total, err := findThreadIDs(
		re.App,
		strings.TrimSpace(query.Get("folder")),
		strings.ToLower(strings.TrimSpace(query.Get("received_for"))),
		page,
		perPage,
	)
	if err != nil {
		return re.InternalServerError("list threads", err)
	}

	items := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		record, err := re.App.FindRecordById("threads", id)
		if err != nil {
			continue
		}
		items = append(items, threadJSON(record))
	}
	return re.JSON(200, pageJSON(items, page, perPage, total))
}

// findThreadIDs pages thread ids newest-first. Membership is decided by the
// thread's messages, never by the denormalized threads.folder: a thread must
// stay in Inbox while any Inbox message exists, even after a reply makes the
// newest message a Sent one. The unfiltered EXISTS also keeps threads whose
// messages have all moved away out of every listing.
func findThreadIDs(app core.App, folder, receivedFor string, page, perPage int) ([]string, int, error) {
	where := "1 = 1"
	params := dbx.Params{"limit": perPage, "offset": (page - 1) * perPage}
	if folder != "" {
		where += " AND m.folder = {:folder}"
		params["folder"] = folder
	}
	if receivedFor != "" {
		where += " AND m.received_for = {:received_for}"
		params["received_for"] = receivedFor
	}
	exists := `EXISTS (
		SELECT 1 FROM messages m
		WHERE m.thread_id = t.id AND ` + where + `
	)`

	var rows []idRow
	if err := app.DB().NewQuery(`
		SELECT t.id
		FROM threads t
		WHERE `+exists+`
		ORDER BY t.last_date DESC
		LIMIT {:limit} OFFSET {:offset}`,
	).Bind(params).All(&rows); err != nil {
		return nil, 0, err
	}
	var count countRow
	if err := app.DB().NewQuery(
		`SELECT COUNT(*) AS total FROM threads t WHERE ` + exists,
	).Bind(params).One(&count); err != nil {
		return nil, 0, err
	}

	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	return ids, count.Total, nil
}

func handleGetThread(re *core.RequestEvent) error {
	id := strings.TrimSpace(re.Request.PathValue("id"))
	thread, err := re.App.FindRecordById("threads", id)
	if err != nil {
		return re.NotFoundError("thread not found", err)
	}
	// Newest-first with a cap, then reversed: an oversized thread keeps its
	// latest replies instead of stopping 500 messages into the past.
	messages, err := re.App.FindRecordsByFilter(
		"messages",
		"thread_id = {:thread}",
		"-date",
		maxThreadMessages,
		0,
		dbx.Params{"thread": id},
	)
	if err != nil {
		return re.InternalServerError("load thread messages", err)
	}
	messageItems := make([]map[string]any, 0, len(messages))
	for i := len(messages) - 1; i >= 0; i-- {
		messageItems = append(messageItems, messageJSON(messages[i]))
	}
	return re.JSON(200, map[string]any{
		"thread":   threadJSON(thread),
		"messages": messageItems,
	})
}

func pagination(pageValue, perPageValue string) (int, int) {
	page, _ := strconv.Atoi(pageValue)
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(perPageValue)
	if perPage < 1 {
		perPage = 50
	}
	if perPage > 200 {
		perPage = 200
	}
	return page, perPage
}

func pageJSON(items any, page, perPage, total int) map[string]any {
	totalPages := 0
	if total > 0 {
		totalPages = int(math.Ceil(float64(total) / float64(perPage)))
	}
	return map[string]any{
		"items":      items,
		"page":       page,
		"perPage":    perPage,
		"totalItems": total,
		"totalPages": totalPages,
	}
}

func threadJSON(record *core.Record) map[string]any {
	return map[string]any{
		"id":                 record.Id,
		"subject":            record.GetString("subject"),
		"normalized_subject": record.GetString("normalized_subject"),
		"snippet":            record.GetString("snippet"),
		"last_date":          record.GetString("last_date"),
		"message_count":      record.GetInt("message_count"),
		"participants":       record.GetString("participants"),
		"received_for":       record.GetString("received_for"),
		"folder":             record.GetString("folder"),
		"unread_count":       record.GetInt("unread_count"),
		"updated_at":         record.GetString("updated_at"),
	}
}

func messageJSON(record *core.Record) map[string]any {
	return map[string]any{
		"id":                 record.Id,
		"account":            record.GetString("account"),
		"folder":             record.GetString("folder"),
		"uid":                record.GetInt("uid"),
		"message_id":         record.GetString("message_id"),
		"subject":            record.GetString("subject"),
		"from_addr":          record.GetString("from_addr"),
		"to_addrs":           record.GetString("to_addrs"),
		"date":               record.GetString("date"),
		"snippet":            record.GetString("snippet"),
		"body_text":          record.GetString("body_text"),
		"body_html":          record.GetString("body_html"),
		"seen":               record.GetBool("seen"),
		"flagged":            record.GetBool("flagged"),
		"in_reply_to":        record.GetString("in_reply_to"),
		"references":         record.GetString("references"),
		"thread_id":          record.GetString("thread_id"),
		"received_for":       record.GetString("received_for"),
		"normalized_subject": record.GetString("normalized_subject"),
	}
}
