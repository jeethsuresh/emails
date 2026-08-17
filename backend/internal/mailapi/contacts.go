package mailapi

import (
	"strings"

	"email.local/backend/internal/mailmeta"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

func handleContacts(re *core.RequestEvent) error {
	query := re.Request.URL.Query()
	q := strings.TrimSpace(query.Get("q"))
	page, perPage := pagination(query.Get("page"), query.Get("perPage"))
	filter := ""
	params := dbx.Params{}
	if q != "" {
		filter = "email ~ {:q} || display_name ~ {:q}"
		params["q"] = q
	}
	records, err := re.App.FindRecordsByFilter("contacts", filter, "-last_message_at", perPage, (page-1)*perPage, params)
	if err != nil {
		return re.InternalServerError("list contacts", err)
	}

	var count countRow
	countSQL := "SELECT COUNT(*) AS total FROM contacts"
	if q != "" {
		countSQL += " WHERE email LIKE '%' || {:q} || '%' COLLATE NOCASE OR display_name LIKE '%' || {:q} || '%' COLLATE NOCASE"
	}
	if err := re.App.DB().NewQuery(countSQL).Bind(params).One(&count); err != nil {
		return re.InternalServerError("count contacts", err)
	}

	items := make([]map[string]any, 0, len(records))
	for _, record := range records {
		items = append(items, contactJSON(record))
	}
	return re.JSON(200, pageJSON(items, page, perPage, count.Total))
}

func handleContactMessages(re *core.RequestEvent) error {
	email := strings.ToLower(strings.TrimSpace(re.Request.PathValue("email")))
	if email == "" {
		return re.BadRequestError("email required", nil)
	}
	query := re.Request.URL.Query()
	page, perPage := pagination(query.Get("page"), query.Get("perPage"))
	records, total, err := findContactMessages(re.App, email, page, perPage)
	if err != nil {
		return re.InternalServerError("list contact messages", err)
	}
	items := make([]map[string]any, 0, len(records))
	for _, record := range records {
		items = append(items, messageJSON(record))
	}
	return re.JSON(200, pageJSON(items, page, perPage, total))
}

// contactFromMatch matches a normalized address against either a bare
// from_addr or the angle-bracket form of a display-name address.
const contactFromMatch = `(
	LOWER(TRIM(from_addr)) = {:email}
	OR LOWER(from_addr) LIKE {:bracketed} ESCAPE '\'
)`

func findContactMessages(app core.App, email string, page, perPage int) ([]*core.Record, int, error) {
	email = mailmeta.NormalizeEmail(email)
	if email == "" {
		return []*core.Record{}, 0, nil
	}
	params := dbx.Params{
		"email":     email,
		"bracketed": "%<" + likeEscape(email) + ">%",
		"limit":     perPage,
		"offset":    (page - 1) * perPage,
	}

	var count countRow
	if err := app.DB().NewQuery(
		`SELECT COUNT(*) AS total FROM messages WHERE ` + contactFromMatch,
	).Bind(params).One(&count); err != nil {
		return nil, 0, err
	}

	// Page over ids in SQL so only the requested page of bodies is hydrated.
	var rows []idRow
	if err := app.DB().NewQuery(`
		SELECT id FROM messages
		WHERE ` + contactFromMatch + `
		ORDER BY date DESC, id DESC
		LIMIT {:limit} OFFSET {:offset}`,
	).Bind(params).All(&rows); err != nil {
		return nil, 0, err
	}

	records := make([]*core.Record, 0, len(rows))
	for _, row := range rows {
		record, err := app.FindRecordById("messages", row.ID)
		if err != nil {
			continue
		}
		records = append(records, record)
	}
	return records, count.Total, nil
}

func likeEscape(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "%", `\%`)
	return strings.ReplaceAll(value, "_", `\_`)
}

func contactJSON(record *core.Record) map[string]any {
	return map[string]any{
		"id":              record.Id,
		"email":           record.GetString("email"),
		"display_name":    record.GetString("display_name"),
		"graph_json":      record.GetString("graph_json"),
		"last_message_at": record.GetString("last_message_at"),
		"message_count":   record.GetInt("message_count"),
		"updated_at":      record.GetString("updated_at"),
	}
}
