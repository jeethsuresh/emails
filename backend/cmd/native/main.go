package main

import (
	"log"
	"os"
	"path/filepath"

	"email.local/backend/internal/analyzer"
	"email.local/backend/internal/calendar"
	"email.local/backend/internal/mailstore"
	"email.local/backend/internal/syncer"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

func main() {
	dataDir := envOr("EMAIL_DATA_DIR", filepath.Join(".", "pb_data"))
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		log.Fatal(err)
	}

	addr := envOr("EMAIL_ADDR", "127.0.0.1:8090")

	app := pocketbase.NewWithConfig(pocketbase.Config{
		DefaultDataDir: dataDir,
	})

	mailstore.Register(app)
	syncer.Register(app)
	analyzer.Register(app)
	calendar.Register(app)

	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		e.Router.BindFunc(func(re *core.RequestEvent) error {
			origin := re.Request.Header.Get("Origin")
			if origin == "" {
				origin = "*"
			}
			re.Response.Header().Set("Access-Control-Allow-Origin", origin)
			re.Response.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			re.Response.Header().Set("Access-Control-Allow-Methods", "GET,POST,PATCH,PUT,DELETE,OPTIONS")
			if re.Request.Method == "OPTIONS" {
				return re.NoContent(204)
			}
			return re.Next()
		})
		log.Printf("email backend listening on http://%s", addr)
		return e.Next()
	})

	os.Args = []string{os.Args[0], "serve", "--http=" + addr, "--dir=" + dataDir}
	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
