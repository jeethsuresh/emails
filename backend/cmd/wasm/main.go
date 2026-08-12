//go:build js && wasm

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"syscall/js"

	"email.local/backend/internal/host"
	"email.local/backend/internal/mailstore"
	"email.local/backend/internal/netbridge"
	"email.local/backend/internal/serve"
	"email.local/backend/internal/syncer"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

func main() {
	dataDir := host.DataDir()
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		log.Fatal(err)
	}

	netbridge.Install()

	pb := pocketbase.NewWithConfig(pocketbase.Config{
		DefaultDev:       false,
		DefaultDataDir:   dataDir,
		DBConnect:        mailstore.DBConnect,
		DataMaxOpenConns: 1,
		DataMaxIdleConns: 1,
		AuxMaxOpenConns:  1,
		AuxMaxIdleConns:  1,
	})

	mailstore.Register(pb)
	syncer.Register(pb)

	pb.OnServe().BindFunc(func(e *core.ServeEvent) error {
		if err := e.Next(); err != nil {
			return err
		}

		handler := e.Server.Handler
		js.Global().Call("ON_POCKETBASE_READY", js.FuncOf(func(this js.Value, args []js.Value) any {
			req := args[0]
			cb := args[1]
			// Must leave the sync JS→Go stack before ServeHTTP; otherwise
			// nested JS (sqlite/wazero) deadlocks under GOOS=js.
			js.Global().Call("setTimeout", js.FuncOf(func(this js.Value, args []js.Value) any {
				handlePB(handler, req, cb)
				return nil
			}), 0)
			return nil
		}))

		exposeAPI(pb)
		host.ReportStatus(host.Status{
			State:   "idle",
			Message: "PocketBase ready",
		})
		return nil
	})

	if err := pb.Bootstrap(); err != nil {
		log.Fatal(err)
	}

	baseURL, err := url.Parse("http://email.local")
	if err != nil {
		log.Fatal(err)
	}

	if err := serve.Serve(pb, baseURL); err != nil {
		log.Fatal(err)
	}
}

func handlePB(handler http.Handler, req js.Value, cb js.Value) {
	res := js.Global().Get("Object").New()
	defer cb.Invoke(res)

	bodyLen := 0
	bodyVal := req.Get("body")
	if bodyVal.Truthy() {
		if bodyVal.Get("byteLength").Truthy() {
			bodyLen = bodyVal.Get("byteLength").Int()
		} else if bodyVal.Get("length").Truthy() {
			bodyLen = bodyVal.Get("length").Int()
		}
	}
	body := make([]byte, bodyLen)
	if bodyLen > 0 {
		js.CopyBytesToGo(body, bodyVal)
	}

	request, err := http.NewRequest(
		req.Get("method").String(),
		req.Get("url").String(),
		bytes.NewReader(body),
	)
	if err != nil {
		res.Set("status", 500)
		res.Set("statusText", err.Error())
		res.Set("headersJSON", "[]")
		res.Set("body", js.Global().Get("Uint8Array").New(0))
		return
	}

	// PocketBase mux matches on path; normalize absolute URLs from the JS client.
	if request.URL != nil && request.URL.Host != "" {
		request.URL.Scheme = ""
		request.URL.Host = ""
		request.RequestURI = ""
	}
	log.Printf("pb %s %s (%d bytes)", request.Method, request.URL.RequestURI(), bodyLen)

	headers := req.Get("headers")
	if headers.Truthy() && headers.Get("length").Truthy() {
		n := headers.Get("length").Int()
		for i := 0; i < n; i++ {
			pair := headers.Index(i)
			request.Header.Add(pair.Index(0).String(), pair.Index(1).String())
		}
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	log.Printf("pb -> %d", recorder.Code)
	_ = setResponse(res, recorder.Result())
}

func setResponse(dst js.Value, src *http.Response) error {
	data, err := io.ReadAll(src.Body)
	if err != nil {
		return err
	}
	body := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(body, data)

	// Flatten headers to a JSON string to survive JS/IPC bridging cleanly.
	type hv struct {
		K string `json:"k"`
		V string `json:"v"`
	}
	var list []hv
	for key, values := range src.Header {
		for _, value := range values {
			list = append(list, hv{K: key, V: value})
		}
	}
	hb, _ := json.Marshal(list)

	dst.Set("status", src.StatusCode)
	dst.Set("statusText", src.Status)
	dst.Set("headersJSON", string(hb))
	dst.Set("body", body)
	return nil
}

func exposeAPI(app core.App) {
	api := js.Global().Get("Object").New()
	api.Set("triggerSync", js.FuncOf(func(this js.Value, args []js.Value) any {
		go syncer.Trigger(app)
		return nil
	}))
	api.Set("saveAccount", js.FuncOf(func(this js.Value, args []js.Value) any {
		raw := args[0].String()
		var m map[string]any
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			return err.Error()
		}
		if err := mailstore.UpsertAccount(app, m); err != nil {
			return err.Error()
		}
		go syncer.Trigger(app)
		return "ok"
	}))
	js.Global().Set("emailAPI", api)
}
