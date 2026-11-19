// Package sources finds the on-disk Sources an agent reads and their load order.
package sources

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"github.com/wlkr-si/agents-view/internal/document"
)

const AgentIDs = "claude, codex, opencode, gemini, cursor, copilot"

type Origin string

const (
	Global     Origin = "Global"
	Project    Origin = "Project"
	AutoMemory Origin = "Auto memory"
	Nested     Origin = "Nested"
)

type Source struct {
	Path     string `json:"path"`
	Origin   Origin `json:"origin"`
	Via      string `json:"via,omitempty"`
	OnDemand bool   `json:"onDemand"`
	Text     string `json:"text"`
	Tokens   int    `json:"tokens"`
	ApplyTo  string `json:"applyTo,omitempty"`
}

type agent struct {
	id      string
	global  func(*discovery)
	project func(*discovery)
}

var agents = []agent{
	{"claude", (*discovery).claudeGlobal, (*discovery).claudeProject},
	{"codex", (*discovery).codexGlobal, (*discovery).codexProject},
	{"opencode", (*discovery).openCodeGlobal, (*discovery).openCodeProject},
	{"gemini", (*discovery).geminiGlobal, (*discovery).geminiProject},
	{"cursor", nil, (*discovery).cursorProject},
	{"copilot", (*discovery).copilotGlobal, (*discovery).copilotProject},
}

// Discover returns Sources in load order, including on-demand Nested Sources.
func Discover(dir, id string) ([]Source, error) {
	for _, agent := range agents {
		if agent.id == id {
			project, err := locateProject(dir)
			if err != nil {
				return nil, err
			}
			return discover(project, agent, true)
		}
	}
	return nil, fmt.Errorf("Unknown agent %q. Choose --agent from %s.", id, AgentIDs)
}

// Readers reports which agents load the working directory's AGENTS.md.
func Readers(dir string) (map[string]bool, error) {
	project, err := locateProject(dir)
	if err != nil {
		return nil, err
	}
	target := filepath.Join(project.dir, "AGENTS.md")
	if resolved, err := filepath.EvalSymlinks(target); err == nil {
		target = resolved
	}
	readers := make(map[string]bool, len(agents))
	for _, agent := range agents {
		files, err := discover(project, agent, false)
		if err != nil {
			return nil, err
		}
		readers[agent.id] = slices.ContainsFunc(files, func(source Source) bool {
			path, err := filepath.EvalSymlinks(source.Path)
			return err == nil && path == target && !source.OnDemand
		})
	}
	return readers, nil
}

// Globals returns the Global Sources used to check Overrides.
func Globals(dir string) ([]Source, error) {
	project, err := locateProject(dir)
	if err != nil {
		return nil, err
	}
	var files []Source
	for _, agent := range agents {
		if agent.global == nil {
			continue
		}
		d := newDiscovery(project, false)
		agent.global(d)
		if agent.id == "opencode" {
			d.openCodeInstructions(true, true)
		}
		if d.err != nil {
			return nil, d.err
		}
		files = append(files, d.files...)
	}
	return files, nil
}

type project struct {
	dir, root, home string
}

func locateProject(dir string) (project, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return project{}, err
	}
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		return project{}, err
	}
	info, err := os.Stat(dir)
	if err != nil {
		return project{}, err
	}
	if !info.IsDir() {
		return project{}, fmt.Errorf("%s is not a directory", dir)
	}
	home, _ := os.UserHomeDir()
	if home == "" {
		if current, err := user.Current(); err == nil {
			home = current.HomeDir
		}
	}
	root := dir
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err == nil {
		if gitRoot, err := filepath.EvalSymlinks(strings.TrimSuffix(string(out), "\n")); err == nil {
			root = gitRoot
		}
	}
	return project{dir: dir, root: root, home: home}, nil
}

// upwards includes both endpoints, from the working directory toward the root.
func upwards(dir, root string) []string {
	var dirs []string
	for {
		dirs = append(dirs, dir)
		parent := filepath.Dir(dir)
		if dir == root || dir == parent {
			return dirs
		}
		dir = parent
	}
}

func envDir(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

type discovery struct {
	project
	nested bool
	files  []Source
	seen   map[string]bool
	err    error
}

func newDiscovery(project project, nested bool) *discovery {
	return &discovery{project: project, nested: nested, files: []Source{}, seen: make(map[string]bool)}
}

func discover(project project, agent agent, nested bool) ([]Source, error) {
	d := newDiscovery(project, nested)
	if agent.global != nil {
		agent.global(d)
	}
	agent.project(d)
	return d.files, d.err
}

func absent(err error) bool {
	return errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTDIR)
}

func (d *discovery) read(path string) (string, bool) {
	if d.err != nil {
		return "", false
	}
	info, err := os.Stat(path)
	if absent(err) || (err == nil && !info.Mode().IsRegular()) {
		return "", false
	}
	if err != nil {
		d.err = document.FileError("stat", err)
		return "", false
	}
	text, err := document.ReadFile(path)
	if absent(err) {
		return "", false
	}
	d.err = err
	return text, err == nil
}

func (d *discovery) add(source Source) bool {
	if d.err != nil {
		return false
	}
	key := source.Path
	if !isURL(key) {
		var err error
		source.Path, err = filepath.Abs(source.Path)
		if err != nil {
			d.err = err
			return false
		}
		key, err = filepath.EvalSymlinks(source.Path)
		if err != nil {
			if !absent(err) {
				d.err = err
			}
			return false
		}
	}
	if d.seen[key] {
		return false
	}
	d.seen[key] = true
	source.OnDemand = source.OnDemand || source.Origin == Nested
	source.Tokens = document.Tokens(source.Text)
	d.files = append(d.files, source)
	return true
}

func (d *discovery) file(path string, origin Origin) {
	if text, exists := d.read(path); exists {
		d.add(Source{Path: path, Origin: origin, Text: text})
	}
}

func (d *discovery) first(dir string, names []string, nonempty bool) (string, string) {
	for _, name := range names {
		path := filepath.Join(dir, name)
		if text, exists := d.read(path); exists && (!nonempty || document.TrimSpace(text) != "") {
			return path, text
		}
	}
	return "", ""
}

// walk visits directories and files in lexical order without following symlinked directories.
func (d *discovery) walk(root string, visit func(path string, entry os.DirEntry)) {
	if d.err != nil {
		return
	}
	// Under a linked Global rules directory, still walk the target, keeping its displayed path.
	if _, err := os.Stat(root); absent(err) {
		return
	} else if err != nil {
		d.err = err
		return
	}
	d.walkEntries(root, visit)
}

func (d *discovery) walkEntries(dir string, visit func(path string, entry os.DirEntry)) {
	entries, err := os.ReadDir(dir)
	if absent(err) {
		return
	}
	if err != nil {
		d.err = err
		return
	}
	for _, entry := range entries {
		if d.err != nil {
			return
		}
		if entry.Name() == ".git" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		visit(path, entry)
		if entry.IsDir() {
			d.walkEntries(path, visit)
		}
	}
}

func (d *discovery) descendants(root string, visit func(string)) {
	if d.nested {
		d.walk(root, func(path string, entry os.DirEntry) {
			if entry.IsDir() {
				visit(path)
			}
		})
	}
}

func (d *discovery) resolve(dir, path string) string {
	switch {
	case strings.HasPrefix(path, "~/"):
		return filepath.Join(d.home, path[2:])
	case filepath.IsAbs(path):
		return filepath.Clean(path)
	default:
		return filepath.Join(dir, path)
	}
}
