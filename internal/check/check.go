package check

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/wlkr-si/agents-view/internal/document"
	"github.com/wlkr-si/agents-view/internal/sources"
)

type Result struct {
	Tokens           int
	Errors, Warnings []string
}

const space = `\t\n\v\f\r\x20\x{A0}\x{1680}\x{2000}-\x{200A}\x{2028}\x{2029}\x{202F}\x{205F}\x{3000}\x{FEFF}`

var readWhen = regexp.MustCompile(`^[Rr][Ee][Aa][Dd] [Ww][Hh][Ee][Nn][` + space + `]+[^` + space + `]`)
var absoluteLink = regexp.MustCompile(`^(?:[\w+.-]+:|/)`)

func Check(file, text string) (Result, error) {
	parsed := document.Parse(text)
	result := Result{Tokens: parsed.Tokens}
	if !parsed.HasVersion {
		result.Errors = append(result.Errors, "Missing first-line "+document.Version+" tag.")
	}
	if parsed.Tokens > document.Budget {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Core exceeds the 1,000-token budget (~%d tokens).", parsed.Tokens))
	}
	names := map[string]int{"Intro": 1}
	for _, section := range parsed.Sections[1:] {
		if names[section.Name] > 0 {
			result.Errors = append(result.Errors, "Duplicate or reserved section name: "+section.Name)
		}
		names[section.Name]++
		if section.Type == document.Pages || section.Name == "Pages" {
			result.checkPages(file, section)
		}
	}
	if names["Overrides"] == 1 {
		entries, err := parsed.OverrideEntries()
		if err != nil {
			return result, err
		}
		if len(entries) > 0 {
			globals, err := sources.Globals(filepath.Dir(file))
			if err != nil {
				return result, err
			}
			for _, entry := range entries {
				if entry.Quote == "" {
					result.Errors = append(result.Errors, "Invalid override: "+entry.Line)
				} else if !containsQuote(globals, entry.Quote) {
					result.Errors = append(result.Errors, "STALE override: "+entry.Quote)
				}
			}
		}
	}
	return result, nil
}

func (result *Result) checkPages(file string, section document.Section) {
	entries := document.PageEntries(section.Body)
	if entries == nil {
		result.Errors = append(result.Errors, "Invalid page list in "+section.Name+".")
	}
	for _, entry := range entries {
		if section.Name == "Pages" && !readWhen.MatchString(entry.Note) {
			result.Errors = append(result.Errors, "Missing read-when note: "+entry.Href)
		}
		if absoluteLink.MatchString(entry.Href) {
			result.Errors = append(result.Errors, "Use a relative Pages link: "+entry.Href)
			continue
		}
		href := entry.Href
		if end := strings.IndexAny(href, "?#"); end >= 0 {
			href = href[:end]
		}
		path, err := url.PathUnescape(href)
		if err != nil || !utf8.ValidString(path) {
			result.Errors = append(result.Errors, "Invalid Pages link: "+entry.Href)
			continue
		}
		switch {
		case path == "":
			path = file
		case !filepath.IsAbs(path):
			path = filepath.Join(filepath.Dir(file), path)
		}
		if _, err := os.Stat(filepath.Clean(path)); err != nil {
			result.Errors = append(result.Errors, "Broken Pages link: "+entry.Href)
		}
	}
}

func containsQuote(globals []sources.Source, quote string) bool {
	for _, source := range globals {
		if strings.Contains(source.Text, quote) {
			return true
		}
	}
	return false
}
