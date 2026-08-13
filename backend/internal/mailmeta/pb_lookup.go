package mailmeta

import (
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

type PBLookup struct {
	App core.App
}

func (p PBLookup) ByMessageID(id string) (string, bool) {
	col, err := p.App.FindCollectionByNameOrId("messages")
	if err != nil {
		return "", false
	}
	rec, err := p.App.FindFirstRecordByFilter(
		col.Id,
		"message_id = {:id} || message_id = {:angle}",
		dbx.Params{"id": id, "angle": "<" + id + ">"},
	)
	if err != nil {
		return "", false
	}
	threadID := rec.GetString("thread_id")
	return threadID, threadID != ""
}

func (p PBLookup) BySubjectParticipants(subject string, participants []string, since string) (string, bool) {
	if subject == "" {
		return "", false
	}
	col, err := p.App.FindCollectionByNameOrId("messages")
	if err != nil {
		return "", false
	}
	rows, err := p.App.FindRecordsByFilter(
		col.Id,
		"normalized_subject = {:subject} && date >= {:since} && thread_id != ''",
		"-date",
		20,
		0,
		dbx.Params{"subject": subject, "since": since},
	)
	if err != nil {
		return "", false
	}
	wanted := make(map[string]struct{}, len(participants))
	for _, participant := range participants {
		if email := NormalizeEmail(participant); email != "" {
			wanted[email] = struct{}{}
		}
	}
	for _, row := range rows {
		rowParticipants := append(ParseAddressList(row.GetString("from_addr")), ParseAddressList(row.GetString("to_addrs"))...)
		for _, participant := range rowParticipants {
			if _, ok := wanted[NormalizeEmail(participant)]; ok {
				return row.GetString("thread_id"), true
			}
		}
	}
	return "", false
}
