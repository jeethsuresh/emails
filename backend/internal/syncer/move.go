package syncer

import (
	"fmt"
	"strings"

	"email.local/backend/internal/netbridge"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// moveRequest is the JSON body accepted by POST /api/email/messages/{id}/move.
// Exactly one of FolderID, FolderName, or ToSpam should be set.
type moveRequest struct {
	FolderID   string `json:"folderId"`
	FolderName string `json:"folderName"`
	ToSpam     bool   `json:"toSpam"`
}

// MoveMessage moves a cached message to a destination mailbox over IMAP
// (UID MOVE, or COPY+STORE+EXPUNGE fallback via go-imap) and updates the
// local message's folder pointer to match. Returns the updated message and
// destination folder records.
func MoveMessage(app core.App, messageID string, req moveRequest) (*core.Record, *core.Record, error) {
	msgCol, err := app.FindCollectionByNameOrId("messages")
	if err != nil {
		return nil, nil, err
	}
	msg, err := app.FindRecordById(msgCol, messageID)
	if err != nil {
		return nil, nil, fmt.Errorf("message not found")
	}

	accountID := msg.GetString("account")
	folderID := msg.GetString("folder")
	uid := uint32(msg.GetFloat("uid"))
	if accountID == "" || folderID == "" || uid == 0 {
		return nil, nil, fmt.Errorf("message missing account/folder/uid")
	}

	accCol, err := app.FindCollectionByNameOrId("accounts")
	if err != nil {
		return nil, nil, err
	}
	acc, err := app.FindRecordById(accCol, accountID)
	if err != nil {
		return nil, nil, fmt.Errorf("account not found")
	}

	folderCol, err := app.FindCollectionByNameOrId("folders")
	if err != nil {
		return nil, nil, err
	}
	srcFolder, err := app.FindRecordById(folderCol, folderID)
	if err != nil {
		return nil, nil, fmt.Errorf("source folder not found")
	}

	dstFolder, err := resolveDestFolder(app, folderCol, accountID, req)
	if err != nil {
		return nil, nil, err
	}
	if dstFolder.Id == srcFolder.Id {
		return msg, dstFolder, nil
	}

	hostName := acc.GetString("imap_host")
	port := int(acc.GetFloat("imap_port"))
	sec := netbridge.ParseSecurity(acc.GetString("imap_security"), acc.GetBool("imap_tls"))
	insecure := acc.GetBool("tls_insecure")
	addr := fmt.Sprintf("%s:%d", hostName, port)

	conn, err := netbridge.Dial("tcp", addr, sec, insecure)
	if err != nil {
		return nil, nil, fmt.Errorf("imap dial: %w", err)
	}

	tlsCfg := netbridge.TLSConfig(hostName, insecure)
	var client *imapclient.Client
	switch sec {
	case netbridge.SecuritySTARTTLS:
		client, err = imapclient.NewStartTLS(conn, &imapclient.Options{TLSConfig: tlsCfg})
		if err != nil {
			_ = conn.Close()
			return nil, nil, fmt.Errorf("imap starttls: %w", err)
		}
	default:
		client = imapclient.New(conn, &imapclient.Options{TLSConfig: tlsCfg})
	}
	defer client.Close()

	if err := client.Login(acc.GetString("username"), acc.GetString("password")).Wait(); err != nil {
		return nil, nil, fmt.Errorf("imap login: %w", err)
	}
	if _, err := client.Select(srcFolder.GetString("name"), nil).Wait(); err != nil {
		return nil, nil, fmt.Errorf("select source folder: %w", err)
	}

	set := imap.UIDSet{}
	set.AddNum(imap.UID(uid))

	// client.Move issues UID MOVE when the server advertises the MOVE
	// extension, otherwise it falls back to UID COPY + STORE \Deleted + EXPUNGE.
	moveData, err := client.Move(set, dstFolder.GetString("name")).Wait()
	if err != nil {
		return nil, nil, fmt.Errorf("imap move: %w", err)
	}

	msg.Set("folder", dstFolder.Id)
	if destUID, ok := singleUID(moveData); ok {
		msg.Set("uid", float64(destUID))
	}
	if err := app.Save(msg); err != nil {
		return nil, nil, fmt.Errorf("save moved message: %w", err)
	}

	logProgress("moved message %s from %s to %s", messageID, srcFolder.GetString("name"), dstFolder.GetString("name"))
	return msg, dstFolder, nil
}

// singleUID extracts the destination UID from a MOVE/COPY response when the
// server supports UIDPLUS and reports exactly one resulting UID.
func singleUID(data *imapclient.MoveData) (uint32, bool) {
	if data == nil || data.DestUIDs == nil {
		return 0, false
	}
	uidSet, ok := data.DestUIDs.(imap.UIDSet)
	if !ok {
		return 0, false
	}
	nums, complete := uidSet.Nums()
	if !complete || len(nums) != 1 {
		return 0, false
	}
	return uint32(nums[0]), true
}

func resolveDestFolder(app core.App, folderCol *core.Collection, accountID string, req moveRequest) (*core.Record, error) {
	switch {
	case req.FolderID != "":
		rec, err := app.FindRecordById(folderCol, req.FolderID)
		if err != nil {
			return nil, fmt.Errorf("destination folder not found")
		}
		if rec.GetString("account") != accountID {
			return nil, fmt.Errorf("destination folder belongs to a different account")
		}
		return rec, nil
	case req.ToSpam:
		return findSpamFolder(app, accountID)
	case strings.TrimSpace(req.FolderName) != "":
		return findFolderByName(app, accountID, req.FolderName)
	default:
		return nil, fmt.Errorf("must specify folderId, folderName, or toSpam")
	}
}

func accountFolders(app core.App, accountID string) ([]*core.Record, error) {
	col, err := app.FindCollectionByNameOrId("folders")
	if err != nil {
		return nil, err
	}
	return app.FindAllRecords(col, dbx.NewExp("account = {:a}", dbx.Params{"a": accountID}))
}

// findFolderByName matches case-insensitive equality first, falling back to
// a substring match. It errors if zero or more than one folder matches at
// whichever match tier is used.
func findFolderByName(app core.App, accountID, name string) (*core.Record, error) {
	target := strings.ToLower(strings.TrimSpace(name))
	if target == "" {
		return nil, fmt.Errorf("empty folder name")
	}
	recs, err := accountFolders(app, accountID)
	if err != nil {
		return nil, err
	}

	var exact, partial []*core.Record
	for _, r := range recs {
		n := strings.ToLower(r.GetString("name"))
		switch {
		case n == target:
			exact = append(exact, r)
		case strings.Contains(n, target):
			partial = append(partial, r)
		}
	}

	switch {
	case len(exact) == 1:
		return exact[0], nil
	case len(exact) > 1:
		return nil, fmt.Errorf("ambiguous folder name %q: %d exact matches", name, len(exact))
	case len(partial) == 1:
		return partial[0], nil
	case len(partial) > 1:
		return nil, fmt.Errorf("ambiguous folder name %q: %d partial matches", name, len(partial))
	default:
		return nil, fmt.Errorf("no folder matches %q", name)
	}
}

// findSpamFolder looks for a spam/junk mailbox by role or name. Folder role
// is currently only populated as inbox/sent/drafts/trash/other by the
// syncer, so this mainly matches on name; role is checked in case a future
// sync marks it explicitly.
func findSpamFolder(app core.App, accountID string) (*core.Record, error) {
	recs, err := accountFolders(app, accountID)
	if err != nil {
		return nil, err
	}

	var candidates []*core.Record
	for _, r := range recs {
		n := strings.ToLower(r.GetString("name"))
		role := strings.ToLower(r.GetString("role"))
		if role == "spam" || role == "junk" || strings.Contains(n, "spam") || strings.Contains(n, "junk") {
			candidates = append(candidates, r)
		}
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no spam/junk folder found for account")
	}

	// Prefer the shortest matching name (top-level "Spam" over nested
	// variants like "Spam/Reviewed").
	best := candidates[0]
	for _, c := range candidates[1:] {
		if len(c.GetString("name")) < len(best.GetString("name")) {
			best = c
		}
	}
	return best, nil
}
