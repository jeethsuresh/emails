package syncer

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
)

type Status struct {
	State          string   `json:"state"`
	Message        string   `json:"message"`
	Phase          string   `json:"phase"`
	CurrentFolder  string   `json:"currentFolder"`
	LastSyncAt     *string  `json:"lastSyncAt"`
	FoldersSynced  int      `json:"foldersSynced"`
	FoldersTotal   int      `json:"foldersTotal"`
	MessagesSynced int      `json:"messagesSynced"`
	Logs           []string `json:"logs"`
}

var (
	statusMu sync.RWMutex
	status   = Status{
		State:   "idle",
		Message: "Idle",
		Phase:   "idle",
	}
)

func CurrentStatus() Status {
	statusMu.RLock()
	defer statusMu.RUnlock()
	out := status
	out.Logs = append([]string(nil), status.Logs...)
	return out
}

func publishLocked() {
	snap := status
	snap.Logs = append([]string(nil), status.Logs...)
	b, _ := json.Marshal(snap)
	log.Printf("EMAIL_STATUS:%s", string(b))
}

func setStatus(mut func(*Status)) {
	statusMu.Lock()
	mut(&status)
	publishLocked()
	statusMu.Unlock()
}

func logProgress(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	statusMu.Lock()
	status.Message = msg
	status.Logs = append(status.Logs, time.Now().Format("15:04:05")+" "+msg)
	if len(status.Logs) > 50 {
		status.Logs = status.Logs[len(status.Logs)-50:]
	}
	publishLocked()
	statusMu.Unlock()
	log.Printf("[sync] %s", msg)
}
