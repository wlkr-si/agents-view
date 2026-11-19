package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/wlkr-si/agents-view/internal/document"
	"github.com/wlkr-si/agents-view/internal/sources"
	"github.com/wlkr-si/agents-view/internal/theme"
	"github.com/wlkr-si/agents-view/web"
)

const (
	editorPolicy = "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; frame-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'"
	editLimit    = 2 * 1024 * 1024 // JSON can expand characters in the buffer.
)

var assets = map[string]struct{ name, contentType string }{
	"/":           {"index.html", "text/html"},
	"/app.js":     {"app.js", "text/javascript"},
	"/syntax.js":  {"syntax.js", "text/javascript"},
	"/style.css":  {"style.css", "text/css"},
	"/theme.js":   {"theme.js", "text/javascript"},
	"/diagram":    {"diagram.html", "text/html"},
	"/diagram.js": {"diagram.js", "text/javascript"},
}

type Source struct {
	Path     string `json:"path"`
	Origin   string `json:"origin"`
	Text     string `json:"text"`
	Via      string `json:"via,omitempty"`
	Tokens   int    `json:"tokens"`
	OnDemand bool   `json:"onDemand,omitempty"`
}

type FindSources func(dir, agent string) ([]Source, error)

type editor struct {
	ctx         context.Context
	dir, file   string
	host        string
	root        *os.Root
	findSources FindSources
	events      events
	watched     watchedFiles
	theme       theme.Store
}

// Serve publishes the Editor address once the listener has bound.
func Serve(ctx context.Context, dir string, findSources FindSources, ready func(string)) error {
	dir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()

	themes, err := theme.Open()
	if err != nil {
		return err
	}

	port, _ := strconv.Atoi(strings.TrimSpace(os.Getenv("AGENTS_VIEW_PORT")))
	listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return err
	}
	defer listener.Close()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	editor := &editor{
		ctx: ctx, dir: dir, file: filepath.Join(dir, "AGENTS.md"),
		host: listener.Addr().String(), root: root, findSources: findSources, theme: themes,
	}
	editor.watched.add(editor.file)
	go editor.watch()
	go func() {
		// Keep the initial Source lookup off the startup path.
		_, _ = editor.sources()
	}()
	server := &http.Server{
		Handler:           editor,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		IdleTimeout:       time.Minute,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}
	finished := make(chan error, 1)
	go func() { finished <- server.Serve(listener) }()
	ready("http://" + editor.host)
	select {
	case err := <-finished:
		cancel()
		editor.events.waitForJob()
		return err
	case <-ctx.Done():
		shutdown, stop := context.WithTimeout(context.Background(), 2*time.Second)
		defer stop()
		err := server.Shutdown(shutdown)
		if err != nil {
			_ = server.Close()
		}
		<-finished
		editor.events.waitForJob()
		return err
	}
}

func (editor *editor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-DNS-Prefetch-Control", "off")

	origin := "http://" + editor.host
	if r.Host != editor.host || (r.Method == http.MethodPost && r.Header.Get("Origin") != origin) {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	path := r.URL.Path
	var err error
	switch r.Method {
	case http.MethodHead:
		if path != mermaidPath {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		fallthrough
	case http.MethodGet:
		if asset, ok := assets[path]; ok {
			policy := editorPolicy
			if path == "/diagram" {
				policy = diagramPolicy(origin)
			}
			h.Set("Content-Security-Policy", policy)
			h.Set("Content-Type", asset.contentType+"; charset=utf-8")
			raw, _ := web.Files.ReadFile(asset.name)
			_, _ = w.Write(raw)
			return
		}
		switch {
		case path == mermaidPath:
			serveMermaid(w, r)
		case path == "/theme.css":
			editor.themeCSS(w)
		case path == "/theme":
			var list theme.List
			list, err = editor.theme.List()
			if err == nil {
				writeJSON(w, http.StatusOK, list)
			}
		case path == "/project":
			err = editor.project(w)
		case path == "/sources":
			err = editor.writeSources(w)
		case path == "/events":
			editor.events.serve(w, r)
		case strings.HasPrefix(path, "/page/"):
			err = editor.page(w, r)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	case http.MethodPost:
		switch path {
		case "/theme":
			err = editor.editTheme(w, r)
		case "/project":
			err = edit(w, r, editor.saveProject)
		case "/preview":
			err = edit(w, r, editor.preview)
		case "/override":
			err = edit(w, r, editor.saveOverride)
		case "/diagram":
			err = edit(w, r, func(data diagramEdit) (any, error) {
				return nil, editor.changeDiagram(data)
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	default:
		w.WriteHeader(http.StatusNotFound)
	}
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, document.ErrChanged) || errors.Is(err, os.ErrNotExist) {
			status = http.StatusConflict
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

type sourceGroup struct {
	Agent string   `json:"agent"`
	Files []Source `json:"files"`
}

func (editor *editor) sources() ([]sourceGroup, error) {
	ids := sources.IDs()
	groups := make([]sourceGroup, 0, len(ids))
	for _, agent := range ids {
		files, err := editor.findSources(editor.dir, agent)
		if err != nil {
			return nil, err
		}
		for index := range files {
			files[index].Tokens = document.Tokens(files[index].Text)
			editor.watched.add(files[index].Path)
		}
		if files == nil {
			files = []Source{}
		}
		groups = append(groups, sourceGroup{agent, files})
	}
	return groups, nil
}

func (editor *editor) writeSources(w http.ResponseWriter) error {
	groups, err := editor.sources()
	if err != nil {
		return err
	}
	readers, err := sources.Readers(editor.dir)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, struct {
		Groups  []sourceGroup   `json:"groups"`
		Readers map[string]bool `json:"readers"`
	}{groups, readers})
	return nil
}

func edit[T any](w http.ResponseWriter, r *http.Request, handle func(T) (any, error)) error {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return errors.New("Expected JSON.")
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, editLimit))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return errors.New("Edit exceeds 2 MiB.")
		}
		return err
	}
	var data T
	if err := json.Unmarshal(raw, &data); err != nil {
		return err
	}
	value, err := handle(data)
	if err != nil {
		return err
	}
	if value == nil {
		value = map[string]bool{"ok": true}
	}
	writeJSON(w, http.StatusOK, value)
	return nil
}
