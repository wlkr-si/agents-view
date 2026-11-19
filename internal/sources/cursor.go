package sources

import (
	"os"
	"path/filepath"
	"slices"
)

func (d *discovery) cursorProject() {
	rules := filepath.Join(d.root, ".cursor", "rules")
	entries, err := os.ReadDir(rules)
	if err != nil && !absent(err) {
		d.err = err
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".mdc" {
			d.file(filepath.Join(rules, entry.Name()), Project)
		}
	}
	d.file(filepath.Join(d.root, "CLAUDE.md"), Project)
	dirs := upwards(d.dir, d.root)
	slices.Reverse(dirs)
	for _, dir := range dirs {
		d.file(filepath.Join(dir, "AGENTS.md"), Project)
	}
	d.descendants(d.dir, func(dir string) {
		d.file(filepath.Join(dir, "AGENTS.md"), Nested)
	})
}
