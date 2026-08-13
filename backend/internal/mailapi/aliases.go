package mailapi

import "github.com/pocketbase/pocketbase/core"

type aliasRow struct {
	Email string `db:"received_for" json:"email"`
	Count int    `db:"n" json:"count"`
}

func handleAliases(re *core.RequestEvent) error {
	rows := make([]aliasRow, 0)
	if err := re.App.DB().NewQuery(`
		SELECT received_for, COUNT(*) AS n
		FROM messages
		WHERE received_for != ''
		GROUP BY received_for
		ORDER BY n DESC, received_for ASC
	`).All(&rows); err != nil {
		return re.InternalServerError("list aliases", err)
	}
	return re.JSON(200, rows)
}
