package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/wlkr-si/agents-view/internal/document"
	"github.com/wlkr-si/agents-view/internal/sources"
)

type section struct {
	Name   string               `json:"name"`
	Type   document.SectionType `json:"type"`
	Line   int                  `json:"line"`
	Tokens int                  `json:"tokens"`
	HTML   string               `json:"html,omitempty"`
	Pages  []pageEntry          `json:"pages,omitempty"`
}

type pageEntry struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Note   string `json:"note"`
	Line   int    `json:"line"`
	Exists bool   `json:"exists"`
}

type override struct {
	Line        int    `json:"line"`
	Quote       string `json:"quote"`
	Replacement string `json:"replacement"`
}

type preview struct {
	Sections  []section  `json:"sections"`
	Overrides []override `json:"overrides"`
}

type project struct {
	Text     string `json:"text"`
	Revision string `json:"revision"`
	File     string `json:"file"`
	preview
}

func (editor *editor) readProject() (project, error) {
	raw, err := os.ReadFile(editor.file)
	if err != nil {
		return project{}, err
	}
	return editor.projectFrom(string(raw)), nil
}

func (editor *editor) projectFrom(raw string) project {
	text := strings.ReplaceAll(document.DecodeUTF8([]byte(raw)), "\r\n", "\n")
	return project{text, document.Revision(raw), editor.file, editor.describe(text)}
}

func (editor *editor) project(w http.ResponseWriter) error {
	project, err := editor.readProject()
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, project)
	return nil
}

type projectEdit struct {
	Text     *string
	Revision string
}

func (editor *editor) saveProject(edit projectEdit) (any, error) {
	if edit.Text == nil {
		return nil, errors.New("Missing AGENTS.md text.")
	}
	text, err := document.Save(editor.file, *edit.Text, edit.Revision)
	if err != nil {
		return nil, err
	}
	return editor.projectFrom(text), nil
}

func (editor *editor) preview(edit projectEdit) (any, error) {
	if edit.Text == nil {
		return nil, errors.New("Missing AGENTS.md text.")
	}
	return editor.describe(*edit.Text), nil
}

func (editor *editor) describe(text string) preview {
	doc := document.Parse(text)
	result := preview{Sections: make([]section, 0, len(doc.Sections))}
	entries, _ := doc.OverrideEntries()
	offset, line := 0, 1
	for _, current := range doc.Sections {
		start := current.Start - len(current.Heading)
		item := section{Name: current.DisplayName(), Type: current.Type, Tokens: current.Tokens}
		// Intro starts at the version line. All other sections start at their heading.
		if current.Heading != "" {
			item.Line = line + strings.Count(text[offset:start], "\n")
		} else {
			item.Line = 1
		}
		line += strings.Count(text[offset:current.BodyStart], "\n")
		offset = current.BodyStart
		if current.Type == document.Pages {
			for _, page := range document.PageEntries(current.Body) {
				item.Pages = append(item.Pages, pageEntry{
					page.Name, page.Href, page.Note, line + page.Line, editor.pageExists(page.Href),
				})
			}
		} else if current.Name != "Overrides" {
			item.HTML = document.RenderHTML(current.Body)
		}
		if current.Name == "Overrides" {
			for _, entry := range entries {
				result.Overrides = append(result.Overrides, override{line + entry.Index, entry.Quote, entry.Replacement})
			}
		}
		result.Sections = append(result.Sections, item)
	}
	return result
}

func (editor *editor) pageExists(href string) bool {
	link, err := url.Parse(href)
	if err != nil || link.IsAbs() || link.Host != "" || strings.HasPrefix(link.Path, "/") {
		return false
	}
	name := filepath.FromSlash(link.Path)
	if name == "" {
		name = "AGENTS.md"
	}
	_, err = editor.root.Stat(name)
	return err == nil
}

func (editor *editor) findSection(name string) (*document.Section, error) {
	body, err := document.ReadFile(editor.file)
	if err != nil {
		return nil, err
	}
	return document.Parse(body).FindSection(name)
}

type overrideEdit struct {
	Quote, Replacement, Revision string
}

func (editor *editor) saveOverride(edit overrideEdit) (any, error) {
	replacement := document.TrimSpace(edit.Replacement)
	if document.TrimSpace(edit.Quote) == "" || replacement == "" ||
		strings.ContainsAny(edit.Quote+edit.Replacement, "\r\n\u2028\u2029") {
		return nil, errors.New("Use one line for the Global text and replacement.")
	}
	groups, err := editor.sources()
	if err != nil {
		return nil, err
	}
	found := false
	for _, group := range groups {
		for _, file := range group.Files {
			if file.Origin != string(sources.Global) {
				continue
			}
			for line := range strings.SplitSeq(strings.ReplaceAll(file.Text, "\r\n", "\n"), "\n") {
				found = found || line == edit.Quote
			}
		}
	}
	if !found {
		return nil, errors.New("The Global line changed on disk.")
	}
	var quote bytes.Buffer
	encoder := json.NewEncoder(&quote)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(edit.Quote)
	text, err := document.AddOverride(editor.file, strings.TrimSuffix(quote.String(), "\n"), replacement, edit.Revision)
	if err != nil {
		return nil, err
	}
	return editor.projectFrom(text), nil
}
