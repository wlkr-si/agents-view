package server

import (
	"errors"
	"fmt"
	"html"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/wlkr-si/agents-view/internal/document"
)

func (editor *editor) page(w http.ResponseWriter, r *http.Request) error {
	relative := strings.TrimPrefix(r.URL.Path, "/page/")
	for _, part := range strings.Split(relative, "/") {
		if strings.HasPrefix(part, ".") || strings.Contains(part, "\\") {
			return errors.New("Choose a visible project page.")
		}
	}
	if strings.HasPrefix(relative, "/") || !filepath.IsLocal(filepath.FromSlash(path.Clean(relative))) {
		return errors.New("Choose a page inside the project.")
	}
	name := path.Clean(relative)
	file, err := editor.root.OpenFile(filepath.FromSlash(name), pageOpenFlags, 0)
	if err != nil {
		return errors.New("Choose a page inside the project.")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	title := info.Name()
	if name == "." {
		title = filepath.Base(editor.dir)
	}
	title = html.EscapeString(title)
	var content string
	if info.IsDir() {
		if !strings.HasSuffix(r.URL.Path, "/") {
			location := url.URL{Path: r.URL.Path + "/", RawQuery: r.URL.RawQuery}
			w.Header().Set("Location", location.String())
			w.WriteHeader(http.StatusPermanentRedirect)
			return nil
		}
		entries, err := file.ReadDir(-1)
		if err != nil {
			return err
		}
		slices.SortFunc(entries, func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
		var list strings.Builder
		fmt.Fprintf(&list, "<h1>%s</h1><ul>", title)
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			href := url.PathEscape(entry.Name())
			switch {
			case entry.IsDir():
				href += "/"
			case entry.Type().IsRegular() && isMarkdown(entry.Name()):
			default:
				continue
			}
			fmt.Fprintf(&list, "<li><a href=\"./%s\">%s</a></li>", html.EscapeString(href), html.EscapeString(entry.Name()))
		}
		list.WriteString("</ul>")
		content = list.String()
	} else {
		if !info.Mode().IsRegular() || !isMarkdown(name) || !isMarkdown(info.Name()) {
			return errors.New("Choose a Markdown page.")
		}
		raw, err := io.ReadAll(file)
		if err != nil {
			return err
		}
		content = document.RenderHTML(document.DecodeUTF8(raw))
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", editorPolicy)
	fmt.Fprintf(w, `<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>%s</title><link rel="stylesheet" href="/theme.css"><link rel="stylesheet" href="/style.css"><main class="page"><p><a href="/">← AGENTS.md</a></p>%s</main></html>`, title, content)
	return nil
}

func isMarkdown(name string) bool {
	return strings.EqualFold(path.Ext(name), ".md")
}
