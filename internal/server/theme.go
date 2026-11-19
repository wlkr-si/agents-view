package server

import (
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/wlkr-si/agents-view/internal/theme"
)

func (editor *editor) themeCSS(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	_, _ = io.WriteString(w, editor.theme.CurrentCSS()+themePickerCSS)
}

func (editor *editor) editTheme(w http.ResponseWriter, r *http.Request) error {
	if r.Header.Get("Content-Type") != "application/toml" {
		return edit(w, r, func(data struct{ Name string }) (any, error) {
			return nil, editor.theme.Use(data.Name)
		})
	}
	file := r.URL.Query().Get("file")
	ext := path.Ext(file)
	if !strings.EqualFold(ext, ".toml") {
		return fmt.Errorf("drop a colors.toml file")
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, theme.MaxSize))
	if err != nil {
		return fmt.Errorf("read colors.toml (limit %d KiB): %w", theme.MaxSize/1024, err)
	}
	if _, err := editor.theme.Install(strings.TrimSuffix(path.Base(file), ext), raw); err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	return nil
}

const themePickerCSS = `
#theme-picker{background:var(--background);color:var(--foreground);border:1px solid var(--muted);border-radius:.5rem;padding:1rem;width:min(20rem,85vw);max-height:80vh}
#theme-picker::backdrop{background:color-mix(in srgb,var(--darker-background) 60%,transparent)}
#theme-picker header{display:flex;align-items:center;justify-content:space-between;gap:1rem}
#theme-picker h2{font:inherit;font-weight:bold;margin:0}
#theme-options{display:grid;gap:.25rem;margin-top:.5rem}
#theme-picker button{font:inherit;color:inherit;background:var(--background);border:1px solid transparent;border-radius:.25rem;padding:.25rem .5rem;cursor:pointer;text-align:start}
#theme-picker button:hover{background:var(--lighter-background)}
#theme-picker button:focus-visible,#theme-picker [aria-current="true"]{border-color:var(--accent)}
#theme-picker button:focus-visible{outline:2px solid var(--accent)}
#theme-picker [aria-current="true"]{font-weight:bold}
`
