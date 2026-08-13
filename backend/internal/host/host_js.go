//go:build js && wasm

package host

import (
	"encoding/json"
	"syscall/js"
)

type Status struct {
	State          string  `json:"state"`
	Message        string  `json:"message"`
	LastSyncAt     *string `json:"lastSyncAt"`
	FoldersSynced  int     `json:"foldersSynced"`
	MessagesSynced int     `json:"messagesSynced"`
}

func DataDir() string {
	h := js.Global().Get("emailHost")
	if h.Truthy() {
		d := h.Get("dataDir")
		if d.Truthy() {
			return d.String()
		}
	}
	return "/pb_data"
}

func AttachmentsDir() string {
	h := js.Global().Get("emailHost")
	if h.Truthy() {
		d := h.Get("attachmentsDir")
		if d.Truthy() {
			return d.String()
		}
	}
	return "/attachments"
}

func ReportStatus(s Status) {
	h := js.Global().Get("emailHost")
	if !h.Truthy() {
		return
	}
	fn := h.Get("reportStatus")
	if !fn.Truthy() {
		return
	}
	b, _ := json.Marshal(s)
	fn.Invoke(string(b))
}

func MimeHeaderGet(raw, name string) string {
	return mimeHeaderGet(raw, name)
}

// Core calls into C WASM via JS glue (emailCore).
func Hash(input string) string {
	core := js.Global().Get("emailCore")
	if !core.Truthy() {
		return ""
	}
	return core.Call("hashBlakeLike", input).String()
}

func Tokenize(text string) []string {
	core := js.Global().Get("emailCore")
	if !core.Truthy() {
		return nil
	}
	arr := core.Call("indexTokenize", text)
	n := arr.Get("length").Int()
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, arr.Index(i).String())
	}
	return out
}

func NormalizeContact(email string) string {
	core := js.Global().Get("emailCore")
	if !core.Truthy() {
		return email
	}
	return core.Call("contactNormalize", email).String()
}

func LoadDBSnapshot(dbPath string) []byte {
	h := js.Global().Get("emailHost")
	if !h.Truthy() {
		return nil
	}
	fn := h.Get("loadDB")
	if !fn.Truthy() {
		return nil
	}
	val := fn.Invoke(dbPath)
	if !val.Truthy() {
		return nil
	}
	n := val.Get("byteLength").Int()
	if n == 0 {
		n = val.Get("length").Int()
	}
	if n == 0 {
		return nil
	}
	out := make([]byte, n)
	js.CopyBytesToGo(out, val)
	return out
}

func SaveDBSnapshot(dbPath string, data []byte) {
	h := js.Global().Get("emailHost")
	if !h.Truthy() {
		return
	}
	fn := h.Get("saveDB")
	if !fn.Truthy() {
		return
	}
	arr := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(arr, data)
	fn.Invoke(dbPath, arr)
}
