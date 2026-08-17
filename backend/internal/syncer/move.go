package syncer

import (
	"fmt"
	"strings"

	"email.local/backend/internal/mailstore"
	"email.local/backend/internal/netbridge"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// moveRequest is the JSON body accepted by POST /api/email/messages/{id}/move.
// Exactly one of FolderID, FolderName, or ToSpam should be set.
type moveRequest struct {
	FolderID     string `json:"folderId"`
	FolderName   string `json:"folderName"`
	ToSpam       bool   `json:"toSpam"`
	CreateFolder bool   `json:"createFolder"`
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

	dstFolder, createName, err := resolveDestFolder(app, folderCol, accountID, req)
	if err != nil {
		return nil, nil, err
	}
	if dstFolder != nil && dstFolder.Id == srcFolder.Id {
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
	if createName != "" {
		if err := createMailboxPath(client, createName); err != nil {
			return nil, nil, fmt.Errorf("create mailbox %q: %w", createName, err)
		}
		dstFolder = core.NewRecord(folderCol)
		dstFolder.Set("account", accountID)
		dstFolder.Set("name", createName)
		dstFolder.Set("role", "other")
		if err := app.Save(dstFolder); err != nil {
			return nil, nil, fmt.Errorf("save new folder: %w", err)
		}
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
	action := "move_to_folder"
	if req.ToSpam {
		action = "move_to_spam"
	}
	mailstore.RecordMailAction(app, messageID, action, dstFolder.GetString("name"))
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

func resolveDestFolder(app core.App, folderCol *core.Collection, accountID string, req moveRequest) (*core.Record, string, error) {
	switch {
	case req.FolderID != "":
		rec, err := app.FindRecordById(folderCol, req.FolderID)
		if err != nil {
			return nil, "", fmt.Errorf("destination folder not found")
		}
		if rec.GetString("account") != accountID {
			return nil, "", fmt.Errorf("destination folder belongs to a different account")
		}
		return rec, "", nil
	case req.ToSpam:
		rec, err := findSpamFolder(app, accountID)
		return rec, "", err
	case strings.TrimSpace(req.FolderName) != "":
		rec, err := findFolderByName(app, accountID, req.FolderName)
		if err == nil {
			return rec, "", nil
		}
		// folderName moves create the mailbox when nothing matches (AI Apply
		// and explicit createFolder). Ambiguous matches still error.
		if !strings.Contains(err.Error(), "no folder matches") {
			return nil, "", err
		}
		if !req.CreateFolder {
			return nil, "", err
		}
		name := strings.TrimSpace(req.FolderName)
		name = strings.ReplaceAll(name, "\\", "/")
		name = strings.Trim(name, "/")
		if name == "" || strings.ContainsAny(name, "\x00\r\n") {
			return nil, "", fmt.Errorf("invalid new folder name")
		}
		folders, ferr := accountFolders(app, accountID)
		if ferr != nil {
			return nil, "", ferr
		}
		return nil, qualifyNewMailboxName(folders, name), nil
	default:
		return nil, "", fmt.Errorf("must specify folderId, folderName, or toSpam")
	}
}

func accountFolders(app core.App, accountID string) ([]*core.Record, error) {
	col, err := app.FindCollectionByNameOrId("folders")
	if err != nil {
		return nil, err
	}
	return app.FindAllRecords(col, dbx.NewExp("account = {:a}", dbx.Params{"a": accountID}))
}

// userFolderNamespace returns the common top-level segment used by existing
// user folders (e.g. "Folders" for Proton-style Folders/travel). Empty if none.
func userFolderNamespace(folders []*core.Record) string {
	counts := map[string]int{}
	for _, f := range folders {
		role := strings.ToLower(strings.TrimSpace(f.GetString("role")))
		if role != "" && role != "other" {
			continue
		}
		name := strings.ReplaceAll(f.GetString("name"), "\\", "/")
		name = strings.Trim(name, "/")
		parts := strings.Split(name, "/")
		if len(parts) < 2 {
			continue
		}
		ns := strings.TrimSpace(parts[0])
		if ns == "" {
			continue
		}
		counts[ns]++
	}
	best, bestN := "", 0
	for ns, n := range counts {
		if n > bestN {
			best, bestN = ns, n
		}
	}
	return best
}

// qualifyNewMailboxName prefixes a new mailbox with the account's user-folder
// namespace when the server rejects top-level creates (Folders/Billing/...).
func qualifyNewMailboxName(folders []*core.Record, name string) string {
	return prefixMailboxWithNamespace(userFolderNamespace(folders), name)
}

// prefixMailboxWithNamespace adds ns/ when name is not already under that namespace.
func prefixMailboxWithNamespace(ns, name string) string {
	name = strings.ReplaceAll(strings.Trim(name, "/"), "\\", "/")
	if name == "" || ns == "" {
		return name
	}
	first, _, _ := strings.Cut(name, "/")
	if strings.EqualFold(first, ns) {
		return name
	}
	return ns + "/" + name
}

// createMailboxPath creates each path segment so nested names like
// Billing/Subscriptions work on servers that require parents first.
func createMailboxPath(client *imapclient.Client, name string) error {
	name = strings.ReplaceAll(strings.Trim(name, "/"), "\\", "/")
	if name == "" {
		return fmt.Errorf("empty mailbox name")
	}
	parts := strings.Split(name, "/")
	path := ""
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if path == "" {
			path = part
		} else {
			path = path + "/" + part
		}
		err := client.Create(path, nil).Wait()
		if err == nil {
			continue
		}
		// Already exists is fine for intermediate and final segments.
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "exists") || strings.Contains(msg, "already") {
			continue
		}
		// Namespace parents (e.g. Folders) often reject CREATE even when they
		// already exist; keep going for non-final segments.
		if i < len(parts)-1 && (strings.Contains(msg, "not allowed") ||
			strings.Contains(msg, "invalid mailbox") ||
			strings.Contains(msg, "permission")) {
			continue
		}
		// Some servers reject creating a parent that already exists as a
		// different error; only fail hard on the final segment.
		if i == len(parts)-1 {
			return err
		}
	}
	return nil
}

// findFolderByName matches case-insensitive equality first, then a path
// suffix / leaf match (so "Billing/Subscriptions" finds
// "Folders/Billing/Subscriptions"), then a unique substring match.
func findFolderByName(app core.App, accountID, name string) (*core.Record, error) {
	target := strings.ToLower(strings.TrimSpace(name))
	target = strings.ReplaceAll(target, "\\", "/")
	target = strings.Trim(target, "/")
	if target == "" {
		return nil, fmt.Errorf("empty folder name")
	}
	recs, err := accountFolders(app, accountID)
	if err != nil {
		return nil, err
	}

	var exact, suffix, leaf, partial []*core.Record
	for _, r := range recs {
		n := strings.ToLower(strings.ReplaceAll(r.GetString("name"), "\\", "/"))
		n = strings.Trim(n, "/")
		switch folderNameMatchKind(n, target) {
		case "exact":
			exact = append(exact, r)
		case "suffix":
			suffix = append(suffix, r)
		case "leaf":
			leaf = append(leaf, r)
		case "partial":
			partial = append(partial, r)
		}
	}

	pick := func(matches []*core.Record, kind string) (*core.Record, error) {
		switch {
		case len(matches) == 1:
			return matches[0], nil
		case len(matches) > 1:
			return nil, fmt.Errorf("ambiguous folder name %q: %d %s matches", name, len(matches), kind)
		default:
			return nil, nil
		}
	}
	if rec, err := pick(exact, "exact"); rec != nil || err != nil {
		return rec, err
	}
	if rec, err := pick(suffix, "path"); rec != nil || err != nil {
		return rec, err
	}
	if rec, err := pick(leaf, "leaf"); rec != nil || err != nil {
		return rec, err
	}
	if rec, err := pick(partial, "partial"); rec != nil || err != nil {
		return rec, err
	}
	return nil, fmt.Errorf("no folder matches %q", name)
}

// folderNameMatchKind classifies how a normalized folder path matches a
// normalized target. Empty string means no match.
func folderNameMatchKind(folder, target string) string {
	if folder == "" || target == "" {
		return ""
	}
	if folder == target {
		return "exact"
	}
	if strings.HasSuffix(folder, "/"+target) {
		return "suffix"
	}
	folderLeaf := folder
	if i := strings.LastIndex(folder, "/"); i >= 0 {
		folderLeaf = folder[i+1:]
	}
	targetLeaf := target
	if i := strings.LastIndex(target, "/"); i >= 0 {
		targetLeaf = target[i+1:]
	}
	if folderLeaf == targetLeaf && targetLeaf != "" {
		return "leaf"
	}
	if strings.Contains(folder, target) {
		return "partial"
	}
	return ""
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
