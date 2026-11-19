package sources

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Copilot documents a depth guard without specifying its value.
var copilotImports = imports{hops: 16, relative: true}

func (d *discovery) copilotGlobal() {
	dir := envDir("COPILOT_HOME", filepath.Join(d.home, ".copilot"))
	d.importedFile(filepath.Join(dir, "copilot-instructions.md"), Global, copilotImports)
	d.copilotRules(filepath.Join(dir, "instructions"), Global)
	for _, path := range strings.Split(os.Getenv("COPILOT_CUSTOM_INSTRUCTIONS_DIRS"), ",") {
		if path = strings.TrimSpace(path); path != "" {
			dir := d.resolve(d.dir, path)
			d.importedFile(filepath.Join(dir, "AGENTS.md"), Global, copilotImports)
			d.copilotRules(dir, Global)
		}
	}
}

func (d *discovery) copilotProject() {
	dirs := upwards(d.dir, d.root)
	nearest := ""
	for _, dir := range dirs {
		path := filepath.Join(dir, "AGENTS.md")
		if _, exists := d.read(path); exists {
			nearest = path
			break
		}
	}
	slices.Reverse(dirs)
	for _, dir := range dirs {
		d.copilotDirectory(dir, Project, nearest == filepath.Join(dir, "AGENTS.md"))
	}
	d.descendants(d.dir, func(dir string) { d.copilotDirectory(dir, Nested, true) })
}

func (d *discovery) copilotDirectory(dir string, origin Origin, agents bool) {
	github := filepath.Join(dir, ".github")
	d.importedFile(filepath.Join(github, "copilot-instructions.md"), origin, copilotImports)
	d.copilotRules(filepath.Join(github, "instructions"), origin)
	if agents {
		d.importedFile(filepath.Join(dir, "AGENTS.md"), origin, copilotImports)
	}
	d.importedFile(filepath.Join(dir, "CLAUDE.md"), origin, copilotImports)
	d.importedFile(filepath.Join(dir, ".claude", "CLAUDE.md"), origin, copilotImports)
	d.file(filepath.Join(dir, "GEMINI.md"), origin)
}

func (d *discovery) copilotRules(dir string, origin Origin) {
	d.walk(dir, func(path string, entry os.DirEntry) {
		if entry.IsDir() || !strings.HasSuffix(path, ".instructions.md") {
			return
		}
		if text, exists := d.read(path); exists {
			applyTo := frontmatterValue(text, "applyTo")
			ruleOrigin := origin
			if applyTo != "" && origin == Project {
				ruleOrigin = Nested
			}
			d.add(Source{Path: path, Origin: ruleOrigin, Text: text, ApplyTo: applyTo, OnDemand: applyTo != ""})
		}
	})
}

func frontmatterValue(text, key string) string {
	first, rest, _ := strings.Cut(text, "\n")
	if strings.TrimSpace(first) != "---" {
		return ""
	}
	value := ""
	for _, line := range strings.Split(rest, "\n") {
		line = strings.TrimSpace(line)
		if line == "---" {
			return value
		}
		if name, field, _ := strings.Cut(line, ":"); name == key {
			value = strings.Trim(strings.TrimSpace(field), `"'`)
		}
	}
	return ""
}
