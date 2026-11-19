package sources

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

const claudeImportHops = 4

var nonAlphanumeric = regexp.MustCompile(`[^a-zA-Z0-9]`)

type claudeSettings struct {
	PluginConfigs map[string]struct {
		Options struct {
			InstructionFiles string
		}
	}
	AutoMemoryEnabled   *bool
	AutoMemoryDirectory string
}

func (d *discovery) claudeDir() string {
	return envDir("CLAUDE_CONFIG_DIR", filepath.Join(d.home, ".claude"))
}

func (d *discovery) claudeSettings(dir string) claudeSettings {
	var settings claudeSettings
	if text, exists := d.read(filepath.Join(dir, "settings.json")); exists {
		_ = json.Unmarshal([]byte(text), &settings)
	}
	return settings
}

func (settings claudeSettings) instructionFiles() string {
	for _, id := range []string{"cc-plugin-agents-md@builtin", "agents-md@builtin"} {
		if value := settings.PluginConfigs[id].Options.InstructionFiles; value != "" {
			return value
		}
	}
	return "claude-md-or-agents-md"
}

func (d *discovery) claudeGlobal() {
	config := d.claudeDir()
	if d.claudeSettings(config).instructionFiles() == "managed-only" {
		return
	}
	d.importedFile(filepath.Join(config, "CLAUDE.md"), Global, imports{hops: claudeImportHops})
	d.walk(filepath.Join(config, "rules"), func(path string, entry os.DirEntry) {
		if !entry.IsDir() && filepath.Ext(path) == ".md" {
			d.importedFile(path, Global, imports{hops: claudeImportHops})
		}
	})
}

func (d *discovery) claudeProject() {
	settings := d.claudeSettings(d.claudeDir())
	mode := settings.instructionFiles()
	rules := imports{hops: claudeImportHops}
	dirs := upwards(d.dir, "")
	slices.Reverse(dirs)
	projectFiles := make([][]Source, len(dirs))
	hasClaude := false
	if mode != "managed-only" {
		for i, dir := range dirs {
			projectFiles[i] = d.claudeFiles(dir, Project)
			hasClaude = hasClaude || len(projectFiles[i]) > 0
		}
		for i, dir := range dirs {
			for _, source := range projectFiles[i] {
				d.addImports(source, rules)
			}
			if mode == "claude-md-and-agents-md" || (!hasClaude && mode != "claude-md") {
				d.claudeAgents(dir, Project)
			}
		}
	}

	d.claudeMemory(settings)
	d.descendants(d.dir, func(dir string) {
		files := d.claudeFiles(dir, Nested)
		for _, source := range files {
			d.addImports(source, rules)
		}
		if mode == "claude-md-and-agents-md" || (len(files) == 0 && mode != "claude-md" && mode != "managed-only") {
			d.claudeAgents(dir, Nested)
		}
	})
}

func (d *discovery) claudeFiles(dir string, origin Origin) []Source {
	var files []Source
	names := []string{"CLAUDE.md", filepath.Join(".claude", "CLAUDE.md")}
	if path, text := d.first(dir, names, false); path != "" {
		// The Global file can also be inside the filesystem-root walk.
		global := filepath.Join(d.claudeDir(), "CLAUDE.md")
		if filepath.Clean(path) != filepath.Clean(global) {
			files = append(files, Source{Path: path, Origin: origin, Text: text})
		}
	}
	if origin == Project {
		path := filepath.Join(dir, "CLAUDE.local.md")
		if text, exists := d.read(path); exists {
			files = append(files, Source{Path: path, Origin: origin, Text: text})
		}
	}
	return files
}

func (d *discovery) claudeAgents(dir string, origin Origin) {
	for _, name := range []string{"AGENTS.md", filepath.Join(".claude", "AGENTS.md")} {
		d.importedFile(filepath.Join(dir, name), origin, imports{hops: claudeImportHops})
	}
}

func (d *discovery) claudeMemory(settings claudeSettings) {
	if os.Getenv("CLAUDE_CODE_DISABLE_AUTO_MEMORY") == "1" ||
		(settings.AutoMemoryEnabled != nil && !*settings.AutoMemoryEnabled) {
		return
	}
	root := d.root
	out, err := exec.Command("git", "-C", d.dir, "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err == nil {
		common := strings.TrimSuffix(string(out), "\n")
		if filepath.Base(common) == ".git" {
			root = filepath.Dir(common)
		}
	}
	slug := envDir("CLAUDE_CODE_PROJECT_DIR_NAME", nonAlphanumeric.ReplaceAllString(root, "-"))
	dir := filepath.Join(d.claudeDir(), "projects", slug, "memory")
	if path := settings.AutoMemoryDirectory; filepath.IsAbs(path) || strings.HasPrefix(path, "~/") {
		dir = d.resolve(d.claudeDir(), path)
	}
	path := filepath.Join(dir, "MEMORY.md")
	if text, exists := d.read(path); exists {
		text = bytePrefix(text, 25*1024)
		lines := strings.SplitAfterN(text, "\n", 201)
		if len(lines) > 200 {
			text = strings.Join(lines[:200], "")
		}
		d.add(Source{Path: path, Origin: AutoMemory, Text: text})
	}
}
