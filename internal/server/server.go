package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/wlkr-si/agents-view/internal/document"
	"github.com/wlkr-si/agents-view/web"
)

const (
	editorPolicy = "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; frame-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'"
	editLimit    = 256_000
)

var assets = map[string]struct{ name, contentType string }{
	"/":           {"index.html", "text/html"},
	"/app.js":     {"app.js", "text/javascript"},
	"/style.css":  {"style.css", "text/css"},
	"/diagram":    {"diagram.html", "text/html"},
	"/diagram.js": {"diagram.js", "text/javascript"},
}

type Source struct {
	Path   string `json:"path"`
	Origin string `json:"origin"`
	Text   string `json:"text"`
	Via    string `json:"via,omitempty"`
	Tokens int    `json:"tokens"`
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
}

// Serve publishes the Editor address once the listener has bound.
func Serve(ctx context.Context, dir string, findSources FindSources, ready func(string)) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()

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
		host: listener.Addr().String(), root: root, findSources: findSources,
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
	case http.MethodGet:
		if asset, ok := assets[path]; ok {
			policy := editorPolicy
			if path == "/diagram" {
				policy = "sandbox allow-scripts; default-src 'none'; script-src " + origin +
					"; style-src 'unsafe-inline'; frame-ancestors " + origin
			}
			h.Set("Content-Security-Policy", policy)
			h.Set("Content-Type", asset.contentType+"; charset=utf-8")
			raw, _ := web.Files.ReadFile(asset.name)
			_, _ = w.Write(raw)
			return
		}
		switch {
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
		case "/section":
			err = edit(w, r, editor.saveSection)
		case "/override":
			err = edit(w, r, editor.saveOverride)
		case "/diagram":
			err = edit(w, r, editor.changeDiagram)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	default:
		w.WriteHeader(http.StatusNotFound)
	}
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

type section struct {
	Name   string               `json:"name"`
	Type   document.SectionType `json:"type"`
	Body   string               `json:"body"`
	Tokens int                  `json:"tokens"`
	HTML   string               `json:"html"`
}

var projectHeading = regexp.MustCompile(`(?m)^#[ \t]+(.+?)\r?$`)

func (editor *editor) readDocument() (document.Document, error) {
	body, err := document.ReadFile(editor.file)
	if err != nil {
		return document.Document{}, err
	}
	return document.Parse(body), nil
}

func (editor *editor) project(w http.ResponseWriter) error {
	doc, err := editor.readDocument()
	if err != nil {
		return err
	}
	name := filepath.Base(editor.dir)
	if heading := projectHeading.FindStringSubmatch(doc.Sections[0].Body); heading != nil {
		name = heading[1]
	}
	sections := make([]section, 0, len(doc.Sections))
	for _, item := range doc.Sections {
		sections = append(sections, section{
			Name: item.DisplayName(), Type: item.Type, Body: item.Body,
			Tokens: item.Tokens, HTML: document.RenderHTML(item.Body),
		})
	}
	writeJSON(w, http.StatusOK, struct {
		Project        string    `json:"project"`
		Directory      string    `json:"directory"`
		Tokens         int       `json:"tokens"`
		MermaidVersion string    `json:"mermaidVersion"`
		Sections       []section `json:"sections"`
	}{name, editor.dir, doc.Tokens, "11.12.2", sections})
	return nil
}

type sourceGroup struct {
	Agent string   `json:"agent"`
	Files []Source `json:"files"`
}

func (editor *editor) sources() ([]sourceGroup, error) {
	groups := make([]sourceGroup, 0, 2)
	for _, agent := range []string{"claude", "codex"} {
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
	doc, err := editor.readDocument()
	if err != nil {
		return err
	}
	entries, err := doc.OverrideEntries()
	if err != nil {
		return err
	}
	groups, err := editor.sources()
	if err != nil {
		return err
	}
	type override struct {
		Line        string `json:"line"`
		Quote       string `json:"quote"`
		Replacement string `json:"replacement"`
		Stale       bool   `json:"stale"`
	}
	overrides := make([]override, 0, len(entries))
	for _, entry := range entries {
		found := entry.Quote != "" && hasGlobal(groups, func(text string) bool {
			return strings.Contains(text, entry.Quote)
		})
		overrides = append(overrides, override{entry.Line, entry.Quote, entry.Replacement, !found})
	}
	writeJSON(w, http.StatusOK, struct {
		Groups    []sourceGroup `json:"groups"`
		Overrides []override    `json:"overrides"`
	}{groups, overrides})
	return nil
}

func hasGlobal(groups []sourceGroup, match func(string) bool) bool {
	for _, group := range groups {
		for _, source := range group.Files {
			if source.Origin == "Global" && match(source.Text) {
				return true
			}
		}
	}
	return false
}

func edit[T any](w http.ResponseWriter, r *http.Request, save func(T) error) error {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return errors.New("Expected JSON.")
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, editLimit))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return errors.New("Edit exceeds 256 KB.")
		}
		return err
	}
	var data T
	if err := json.Unmarshal(raw, &data); err != nil {
		return err
	}
	if err := save(data); err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	return nil
}

type sectionEdit struct {
	Name, Body, Expected *string
}

func (editor *editor) saveSection(edit sectionEdit) error {
	if edit.Name == nil || edit.Body == nil || edit.Expected == nil {
		return errors.New("Invalid section edit.")
	}
	current, err := editor.findSection(*edit.Name)
	if err != nil {
		return err
	}
	if current == nil || current.Type == document.Diagram {
		return errors.New("Choose an editable section.")
	}
	return document.Update(editor.file, *edit.Name, *edit.Body, edit.Expected, false)
}

func (editor *editor) findSection(name string) (*document.Section, error) {
	doc, err := editor.readDocument()
	if err != nil {
		return nil, err
	}
	return doc.FindSection(name)
}

type overrideEdit struct {
	Quote, Replacement string
}

func (editor *editor) saveOverride(edit overrideEdit) error {
	replacement := document.TrimSpace(edit.Replacement)
	if document.TrimSpace(edit.Quote) == "" || replacement == "" ||
		strings.ContainsAny(edit.Quote+edit.Replacement, "\r\n") {
		return errors.New("Use one line for the quote and replacement.")
	}
	groups, err := editor.sources()
	if err != nil {
		return err
	}
	if !hasGlobal(groups, func(text string) bool {
		for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
			if line == edit.Quote {
				return true
			}
		}
		return false
	}) {
		return errors.New("The global line changed on disk.")
	}
	var quote bytes.Buffer
	encoder := json.NewEncoder(&quote)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(edit.Quote)
	body := fmt.Sprintf("- Global: %s → here: %s", strings.TrimSuffix(quote.String(), "\n"), replacement)
	return document.Update(editor.file, "Overrides", body, nil, true)
}
