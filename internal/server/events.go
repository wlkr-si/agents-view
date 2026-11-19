package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"
)

type event struct {
	name, data string
}

type events struct {
	mu      sync.Mutex
	streams map[chan event]struct{}
	job     string
	running <-chan struct{}
}

func (events *events) broadcast(update event) {
	events.mu.Lock()
	defer events.mu.Unlock()
	events.broadcastLocked(update)
}

func (events *events) broadcastLocked(update event) {
	for stream := range events.streams {
		select {
		case stream <- update:
		default:
			// Let a slow browser reconnect without blocking writes or file polling.
			close(stream)
			delete(events.streams, stream)
		}
	}
}

func (events *events) serve(w http.ResponseWriter, r *http.Request) {
	stream := make(chan event, 16)
	events.mu.Lock()
	if events.streams == nil {
		events.streams = make(map[chan event]struct{})
	}
	events.streams[stream] = struct{}{}
	stream <- event{"job", events.job}
	if r.Header.Get("Last-Event-ID") != "" {
		stream <- event{"change", ""}
	}
	events.mu.Unlock()
	defer func() {
		events.mu.Lock()
		delete(events.streams, stream)
		events.mu.Unlock()
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	controller := http.NewResponseController(w)
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		var message string
		select {
		case <-r.Context().Done():
			return
		case update, ok := <-stream:
			if !ok {
				return
			}
			data, _ := json.Marshal(update.data)
			message = fmt.Sprintf("id: editor\nevent: %s\ndata: %s\n\n", update.name, data)
		case <-heartbeat.C:
			message = ": keep-alive\n\n"
		}
		_ = controller.SetWriteDeadline(time.Now().Add(time.Second))
		if _, err := fmt.Fprint(w, message); err != nil {
			return
		}
		if err := controller.Flush(); err != nil {
			return
		}
	}
}

type watchedFiles struct {
	mu    sync.Mutex
	files map[string]os.FileInfo
}

func (watch *watchedFiles) add(path string) {
	watch.mu.Lock()
	defer watch.mu.Unlock()
	if watch.files == nil {
		watch.files = make(map[string]os.FileInfo)
	}
	if _, known := watch.files[path]; !known {
		watch.files[path], _ = os.Stat(path)
	}
}

func (watch *watchedFiles) changed() bool {
	watch.mu.Lock()
	defer watch.mu.Unlock()
	changed := false
	for path, previous := range watch.files {
		current, _ := os.Stat(path)
		watch.files[path] = current
		switch {
		case current == nil || previous == nil:
			changed = changed || current != previous
		case !os.SameFile(previous, current), previous.Size() != current.Size(),
			!previous.ModTime().Equal(current.ModTime()):
			changed = true
		}
	}
	return changed
}

func (editor *editor) watch() {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-editor.ctx.Done():
			return
		case <-ticker.C:
			if editor.watched.changed() {
				editor.events.broadcast(event{"change", ""})
			}
		}
	}
}
