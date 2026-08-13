package mailapi

import (
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

func handleContacts(re *core.RequestEvent) error {
	query := re.Request.URL.Query()
	q := strings.TrimSpace(query.Get("q"))
	page, perPage := pagination(query.Get("page"), query.Get("perPage"))
	filter := "id != ''"
	params := dbx.Params{}
	if q != "" {
		filter += " && (email ~ {:q} || display_name ~ {:q})"
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
	params := dbx.Params{"email": email}
	records, err := re.App.FindRecordsByFilter(
		"messages",
		"from_addr = {:email}",
		"-date",
		perPage,
		(page-1)*perPage,
		params,
	)
	if err != nil {
		return re.InternalServerError("list contact messages", err)
	}
	var count countRow
	if err := re.App.DB().NewQuery(
		"SELECT COUNT(*) AS total FROM messages WHERE from_addr = {:email}",
	).Bind(params).One(&count); err != nil {
		return re.InternalServerError("count contact messages", err)
	}
	items := make([]map[string]any, 0, len(records))
	for _, record := range records {
		items = append(items, messageJSON(record))
	}
	return re.JSON(200, pageJSON(items, page, perPage, count.Total))
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
