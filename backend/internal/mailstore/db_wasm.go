//go:build js && wasm

package mailstore

import (
	"path/filepath"
	"strings"

	"email.local/backend/internal/host"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
	"github.com/ncruces/go-sqlite3/vfs/memdb"
	"github.com/pocketbase/dbx"
)

var dbs = map[string]*dbx.DB{}

func DBConnect(dbPath string) (*dbx.DB, error) {
	if db, ok := dbs[dbPath]; ok {
		return db, nil
	}

	name := "/" + strings.TrimPrefix(filepath.Base(dbPath), "/")
	if snap := host.LoadDBSnapshot(dbPath); len(snap) > 0 {
		memdb.Create(name, snap)
	}

	db, err := dbx.Open("sqlite3", "file:"+name+"?vfs=memdb")
	if err != nil {
		return nil, err
	}
	dbs[dbPath] = db
	return db, nil
}
