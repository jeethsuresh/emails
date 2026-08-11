package analyzer

import (
	"log"
	"sync"
)

// Status mirrors the syncer status shape but for the analyzer worker.
type Status struct {
	State            string `json:"state"` // idle | running | paused
	QueueDepth       int    `json:"queueDepth"`
	CurrentMessageID string `json:"currentMessageId"`
	Message          string `json:"message"`
	Model            string `json:"model"`
}

var (
	statusMu sync.RWMutex
	status   = Status{State: "idle"}
)

// CurrentStatus returns a snapshot of the analyzer worker's current status.
func CurrentStatus() Status {
	statusMu.RLock()
	defer statusMu.RUnlock()
	return status
}

func setStatus(mut func(*Status)) {
	statusMu.Lock()
	mut(&status)
	statusMu.Unlock()
}

func logProgress(format string, args ...any) {
	log.Printf("[analyzer] "+format, args...)
}
