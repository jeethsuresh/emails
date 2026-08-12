package serve

import (
	"context"
	"crypto/tls"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"golang.org/x/crypto/acme"
)

// Serve builds the PocketBase HTTP stack without listening on a port.
// Electron talks to the handler in-process via WASM JS callbacks.
func Serve(app core.App, baseURL *url.URL) error {
	if err := app.RunAllMigrations(); err != nil {
		return err
	}

	pbRouter, err := apis.NewRouter(app)
	if err != nil {
		return err
	}

	baseCtx, cancelBaseCtx := context.WithCancel(context.Background())
	defer cancelBaseCtx()

	server := &http.Server{
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			NextProtos: []string{acme.ALPNProto},
		},
		WriteTimeout:      5 * time.Minute,
		ReadTimeout:       5 * time.Minute,
		ReadHeaderTimeout: 1 * time.Minute,
		Addr:              baseURL.Host,
		BaseContext: func(l net.Listener) context.Context {
			return baseCtx
		},
		ErrorLog: log.New(&serverErrorLogWriter{app: app}, "", 0),
	}

	serveEvent := new(core.ServeEvent)
	serveEvent.App = app
	serveEvent.Router = pbRouter
	serveEvent.Server = server
	serveEvent.InstallerFunc = apis.DefaultInstallerFunc

	if err := app.OnServe().Trigger(serveEvent, func(e *core.ServeEvent) error {
		handler, err := e.Router.BuildMux()
		if err != nil {
			return err
		}
		e.Server.Handler = handler

		if e.InstallerFunc != nil {
			// Skip interactive installer in embedded desktop mode.
			e.InstallerFunc = nil
		}
		return nil
	}); err != nil {
		return err
	}

	log.Printf("PocketBase in-process at %s", baseURL)
	wait := make(chan struct{})
	<-wait
	return nil
}

type serverErrorLogWriter struct {
	app core.App
}

func (s *serverErrorLogWriter) Write(p []byte) (int, error) {
	s.app.Logger().Debug(strings.TrimSpace(string(p)))
	return len(p), nil
}
