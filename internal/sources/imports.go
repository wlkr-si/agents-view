package sources

import (
	"path/filepath"
	"strings"

	"github.com/wlkr-si/agents-view/internal/document"
)

type imports struct {
	hops     int
	boundary string
	relative bool
}

func (d *discovery) importedFile(path string, origin Origin, rules imports) {
	if text, exists := d.read(path); exists {
		d.addImports(Source{Path: path, Origin: origin, Text: text}, rules)
	}
}

func (d *discovery) addImports(source Source, rules imports) {
	if !d.add(source) || rules.hops == 0 {
		return
	}
	rules.hops--
	for _, target := range document.Imports(source.Text) {
		if rules.relative && (filepath.IsAbs(target) || strings.HasPrefix(target, "~/")) {
			continue
		}
		path := d.resolve(filepath.Dir(source.Path), target)
		if rules.boundary != "" && !inside(rules.boundary, path) {
			continue
		}
		if text, exists := d.read(path); exists {
			d.addImports(Source{Path: path, Origin: source.Origin, Via: source.Path, Text: text}, rules)
		}
	}
}

func inside(root, path string) bool {
	root, rootErr := filepath.EvalSymlinks(root)
	path, pathErr := filepath.EvalSymlinks(path)
	if rootErr != nil || pathErr != nil {
		return false
	}
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
