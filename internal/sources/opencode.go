package sources

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/wlkr-si/agents-view/internal/document"
)

func (d *discovery) openCodeDir() string {
	return filepath.Join(envDir("XDG_CONFIG_HOME", filepath.Join(d.home, ".config")), "opencode")
}

func openCodeNames(agents, claude string) []string {
	if os.Getenv("OPENCODE_DISABLE_CLAUDE_CODE_PROMPT") == "1" || os.Getenv("OPENCODE_DISABLE_CLAUDE_CODE") == "1" {
		return []string{agents}
	}
	return []string{agents, claude}
}

func (d *discovery) openCodeGlobal() {
	config := envDir("OPENCODE_CONFIG_DIR", d.openCodeDir())
	names := openCodeNames(filepath.Join(config, "AGENTS.md"), filepath.Join(d.home, ".claude", "CLAUDE.md"))
	for _, path := range names {
		if text, exists := d.read(path); exists {
			d.add(Source{Path: path, Origin: Global, Text: text})
			break
		}
	}
}

func (d *discovery) openCodeProject() {
	names := openCodeNames("AGENTS.md", "CLAUDE.md")
	add := func(dir string, origin Origin) {
		if path, text := d.first(dir, names, false); path != "" {
			d.add(Source{Path: path, Origin: origin, Text: text})
		}
	}
	if os.Getenv("OPENCODE_DISABLE_PROJECT_CONFIG") != "1" {
		for _, dir := range upwards(d.dir, d.root) {
			add(dir, Project)
		}
	}
	d.openCodeInstructions(false, d.fetchURLs)
	if os.Getenv("OPENCODE_DISABLE_PROJECT_CONFIG") != "1" {
		d.descendants(d.dir, func(dir string) { add(dir, Nested) })
	}
}

func (d *discovery) openCodeInstructions(globalOnly, fetchURLs bool) {
	globalPaths := []string{}
	projectPaths := []string{}
	for _, name := range []string{"opencode.json", "opencode.jsonc"} {
		globalPaths = append(globalPaths, filepath.Join(d.openCodeDir(), name))
	}
	if path := os.Getenv("OPENCODE_CONFIG"); path != "" {
		globalPaths = append(globalPaths, path)
	}
	dirs := upwards(d.dir, d.root)
	if os.Getenv("OPENCODE_DISABLE_PROJECT_CONFIG") == "1" {
		dirs = []string{envDir("OPENCODE_CONFIG_DIR", d.openCodeDir())}
	} else if !globalOnly {
		rootFirst := slices.Clone(dirs)
		slices.Reverse(rootFirst)
		for _, dir := range rootFirst {
			projectPaths = append(projectPaths, filepath.Join(dir, "opencode.json"), filepath.Join(dir, "opencode.jsonc"))
		}
		for _, dir := range rootFirst {
			projectPaths = append(projectPaths, filepath.Join(dir, ".opencode", "opencode.json"), filepath.Join(dir, ".opencode", "opencode.jsonc"))
		}
	}
	if dir := os.Getenv("OPENCODE_CONFIG_DIR"); dir != "" {
		// This configuration is shared across projects.
		globalPaths = append(globalPaths, filepath.Join(dir, "opencode.json"), filepath.Join(dir, "opencode.jsonc"))
	}
	for _, path := range globalPaths {
		d.openCodeConfig(path, Global, dirs, globalOnly, fetchURLs)
	}
	for _, path := range projectPaths {
		d.openCodeConfig(path, Project, dirs, globalOnly, fetchURLs)
	}
}

func (d *discovery) openCodeConfig(path string, origin Origin, dirs []string, globalOnly, fetchURLs bool) {
	var config struct{ Instructions []string }
	if text, exists := d.read(path); !exists || json.Unmarshal(jsonc(text), &config) != nil {
		return
	}
	for _, pattern := range config.Instructions {
		switch {
		case isURL(pattern):
			if fetchURLs {
				d.openCodeURL(pattern, origin)
			}
		case filepath.IsAbs(pattern), strings.HasPrefix(pattern, "~/"):
			d.openCodeGlob(d.resolve(d.dir, pattern), origin)
		case pattern != "" && !globalOnly:
			for _, dir := range dirs {
				d.openCodeGlob(filepath.Join(dir, pattern), Project)
			}
		}
	}
}

// JSONC adds comments and trailing commas to JSON.
var jsoncTokens = regexp.MustCompile(`"(?:[^"\\]|\\.)*"|//[^\r\n]*|/\*(?s:.*?)\*/|,|[^"/,]+|/`)

func jsonc(text string) []byte {
	var result strings.Builder
	var comma bool
	for _, token := range jsoncTokens.FindAllString(text, -1) {
		switch {
		case strings.HasPrefix(token, "//"), strings.HasPrefix(token, "/*"):
			result.WriteByte(' ')
		case token == ",":
			comma = true
		default:
			trimmed := strings.TrimSpace(token)
			if comma && trimmed != "" {
				if trimmed[0] != '}' && trimmed[0] != ']' {
					result.WriteByte(',')
				}
				comma = false
			}
			result.WriteString(token)
		}
	}
	return []byte(result.String())
}

func (d *discovery) openCodeGlob(pattern string, origin Origin) {
	// filepath.Glob covers ordinary patterns. A complete ** segment may span directories.
	separator := string(filepath.Separator)
	segments := strings.Split(pattern, separator)
	for index, segment := range segments {
		if segment != "**" {
			continue
		}
		prefix := strings.Join(segments[:index], separator)
		if prefix == "" {
			prefix = separator
		}
		suffix := strings.Join(segments[index+1:], separator)
		bases, _ := filepath.Glob(prefix)
		for _, base := range bases {
			if suffix != "" {
				d.openCodeGlob(filepath.Join(base, suffix), origin)
			}
			d.walk(base, func(path string, entry os.DirEntry) {
				if suffix == "" && !entry.IsDir() {
					d.file(path, origin)
				} else if suffix != "" && entry.IsDir() {
					d.openCodeGlob(filepath.Join(path, suffix), origin)
				}
			})
		}
		return
	}
	if _, err := os.Stat(pattern); err == nil {
		d.file(pattern, origin)
		return
	}
	matches, _ := filepath.Glob(pattern)
	for _, path := range matches {
		d.file(path, origin)
	}
}

func isURL(path string) bool {
	return strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://")
}

var openCodeHTTP = &http.Client{Timeout: 5 * time.Second}

func (d *discovery) openCodeURL(url string, origin Origin) {
	if d.err != nil || d.seen[url] {
		return
	}
	response, err := openCodeHTTP.Get(url)
	if err != nil {
		return
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return
	}
	body, err := io.ReadAll(response.Body)
	if err == nil {
		d.add(Source{Path: url, Origin: origin, Text: document.DecodeUTF8(body)})
	}
}
