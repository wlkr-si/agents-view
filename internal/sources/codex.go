package sources

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

var codexNames = []string{"AGENTS.override.md", "AGENTS.md"}

func (d *discovery) codexDir() string {
	return envDir("CODEX_HOME", filepath.Join(d.home, ".codex"))
}

func (d *discovery) codexGlobal() {
	if path, text := d.first(d.codexDir(), codexNames, true); path != "" {
		d.add(Source{Path: path, Origin: Global, Text: text})
	}
}

func (d *discovery) codexProject() {
	settings, _ := d.read(filepath.Join(d.codexDir(), "config.toml"))
	names := slices.Clone(codexNames)
	for _, name := range tomlStrings(tomlValue(settings, "", "project_doc_fallback_filenames")) {
		if name != "" && name != "." && name != ".." && filepath.Base(name) == name {
			names = append(names, name)
		}
	}
	remaining := 32 * 1024
	if limit, err := strconv.Atoi(strings.ReplaceAll(tomlValue(settings, "", "project_doc_max_bytes"), "_", "")); err == nil && limit >= 0 {
		remaining = limit
	}
	dirs := upwards(d.dir, d.root)
	slices.Reverse(dirs)
	for _, dir := range dirs {
		if remaining == 0 {
			break
		}
		if path, text := d.first(dir, names, true); path != "" {
			prefix := bytePrefix(text, remaining)
			remaining -= min(remaining, len(text))
			d.add(Source{Path: path, Origin: Project, Text: prefix})
		}
	}
	if tomlValue(settings, "features", "memories") == "true" && tomlValue(settings, "memories", "use_memories") != "false" {
		d.file(filepath.Join(d.codexDir(), "memories_v2", "memory_summary.md"), AutoMemory)
	}
}

func bytePrefix(text string, limit int) string {
	end := min(len(text), limit)
	for end < len(text) && !utf8.RuneStart(text[end]) {
		end--
	}
	return text[:end]
}

// Read only the scalar and string-array values used by Sources discovery.
var tomlToken = regexp.MustCompile(`"""(?s:.*?)"""|'''(?s:.*?)'''|"(?:[^"\\\n]|\\.)*"|'[^'\n]*'|#[^\n]*|[\[\]=,\n]|[^\s\[\]=,#"']+`)

func tomlValue(text, table, key string) string {
	tokens := tomlToken.FindAllString(text, -1)
	current := ""
	for index := 0; index < len(tokens); index++ {
		token := tokens[index]
		if token == "\n" || strings.HasPrefix(token, "#") {
			continue
		}
		if token == "[" {
			start := index + 1
			for index < len(tokens) && tokens[index] != "\n" {
				index++
			}
			header := strings.Join(tokens[start:index], "")
			header, _, _ = strings.Cut(header, "#")
			current = strings.TrimSuffix(header, "]")
			continue
		}
		matches := current == table && strings.Trim(token, `"'`) == key
		start := index + 1
		if start < len(tokens) && tokens[start] == "=" {
			start++
		}
		index = start
		if start < len(tokens) && tokens[start] == "[" {
			depth := 1
			for index++; index < len(tokens) && depth > 0; index++ {
				switch tokens[index] {
				case "[":
					depth++
				case "]":
					depth--
				}
			}
		}
		for index < len(tokens) && tokens[index] != "\n" && !strings.HasPrefix(tokens[index], "#") {
			index++
		}
		if matches {
			return strings.Join(tokens[start:index], " ")
		}
	}
	return ""
}

func tomlStrings(value string) []string {
	tokens := tomlToken.FindAllString(value, -1)
	if len(tokens) < 2 || tokens[0] != "[" || tokens[len(tokens)-1] != "]" {
		return nil
	}
	var names []string
	for _, token := range tokens[1 : len(tokens)-1] {
		switch token[0] {
		case '"':
			var name string
			if json.Unmarshal([]byte(token), &name) == nil {
				names = append(names, name)
			}
		case '\'':
			names = append(names, token[1:len(token)-1])
		}
	}
	return names
}
