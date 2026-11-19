package sources

import (
	"encoding/json"
	"path/filepath"
)

const geminiImportHops = 5

func (d *discovery) geminiDir() string {
	return filepath.Join(envDir("GEMINI_CLI_HOME", d.home), ".gemini")
}

func (d *discovery) geminiNames() []string {
	names := []string{"GEMINI.md"}
	dirs := []string{d.geminiDir(), filepath.Join(d.root, ".gemini")}
	if d.dir != d.root {
		dirs = append(dirs, filepath.Join(d.dir, ".gemini"))
	}
	for _, dir := range dirs {
		var settings struct {
			Context struct{ FileName json.RawMessage }
		}
		text, exists := d.read(filepath.Join(dir, "settings.json"))
		if !exists || json.Unmarshal([]byte(text), &settings) != nil {
			continue
		}
		var list []string
		var name string
		switch {
		case json.Unmarshal(settings.Context.FileName, &list) == nil && list != nil:
			names = list
		case json.Unmarshal(settings.Context.FileName, &name) == nil && name != "":
			names = []string{name}
		}
	}
	valid := names[:0]
	for _, name := range names {
		if name != "" && name != "." && name != ".." && name == filepath.Base(name) {
			valid = append(valid, name)
		}
	}
	return valid
}

func (d *discovery) geminiGlobal() {
	dir := d.geminiDir()
	for _, name := range d.geminiNames() {
		d.importedFile(filepath.Join(dir, name), Global, imports{hops: geminiImportHops, boundary: dir})
	}
}

func (d *discovery) geminiProject() {
	names := d.geminiNames()
	add := func(dir string, origin Origin) {
		for _, name := range names {
			d.importedFile(filepath.Join(dir, name), origin, imports{hops: geminiImportHops, boundary: d.root})
		}
	}
	for _, dir := range upwards(d.dir, d.root) {
		add(dir, Project)
	}
	d.descendants(d.dir, func(dir string) { add(dir, Nested) })
}
