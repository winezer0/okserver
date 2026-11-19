package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Each connection reads durable task snapshots. Reconnects recover current state,
// including work acknowledged before a server restart; no transient event log is required.
func (fs *fileServer) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, 405, "method not allowed", nil)
		return
	}
	if fs.uploads == nil {
		writeAPIError(w, 503, "upload service unavailable", nil)
		return
	}
	if fs.eventClients.Add(1) > 16 {
		fs.eventClients.Add(-1)
		writeAPIError(w, 429, "too many event streams", nil)
		return
	}
	defer fs.eventClients.Add(-1)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	controller := http.NewResponseController(w)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var previous []byte
	sequence := 0
	for {
		data, err := json.Marshal(map[string]any{"uploads": publicUploads(fs.uploads.Tasks()), "version": 1})
		if err != nil {
			return
		}
		_ = controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if !bytes.Equal(data, previous) {
			sequence++
			_, err = fmt.Fprintf(w, "id: %d\nevent: tasks\ndata: %s\n\n", sequence, data)
			previous = data
		} else {
			_, err = fmt.Fprint(w, ": heartbeat\n\n")
		}
		if err != nil || controller.Flush() != nil {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}
