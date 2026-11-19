package check

import (
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/wlkr-si/agents-view/internal/document"
)

// Global represents a Global file already used by check.
type Global struct {
	Path, Text, Via string
}

func globalFiles() ([]string, error) {
	var files []string
	for _, agent := range []string{"claude", "codex"} {
		globals, err := Globals(agent)
		if err != nil {
			return nil, err
		}
		for _, file := range globals {
			files = append(files, file.Text)
		}
	}
	return files, nil
}

// Globals returns the Global files for the agents supported by check.
func Globals(agent string) ([]Global, error) {
	home, _ := os.UserHomeDir()
	if home == "" {
		if current, err := user.Current(); err == nil {
			home = current.HomeDir
		}
	}
	if agent == "claude" {
		config := os.Getenv("CLAUDE_CONFIG_DIR")
		if config == "" {
			config = filepath.Join(home, ".claude")
		}
		globals := claudeGlobals{seen: make(map[string]bool), home: home}
		err := globals.add(filepath.Join(config, "CLAUDE.md"), "", 0)
		return globals.files, err
	}
	if agent != "codex" {
		return nil, nil
	}

	codex := os.Getenv("CODEX_HOME")
	if codex == "" {
		codex = filepath.Join(home, ".codex")
	}
	for _, name := range []string{"AGENTS.override.md", "AGENTS.md"} {
		path := filepath.Join(codex, name)
		text, _, err := readGlobal(path)
		if err != nil {
			return nil, err
		}
		if document.TrimSpace(text) != "" {
			return []Global{{Path: path, Text: text}}, nil
		}
	}
	return nil, nil
}

type claudeGlobals struct {
	files []Global
	seen  map[string]bool
	home  string
}

func (globals *claudeGlobals) add(path, via string, depth int) error {
	text, exists, err := readGlobal(path)
	if err != nil || !exists {
		return err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	key, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return document.FileError("lstat", err)
	}
	if globals.seen[key] {
		return nil
	}
	globals.seen[key] = true
	globals.files = append(globals.files, Global{Path: absolute, Text: text, Via: via})
	if depth >= 5 {
		return nil
	}

	for _, target := range document.Imports(text) {
		switch {
		case strings.HasPrefix(target, "~/"):
			target = filepath.Join(globals.home, target[2:])
		case !filepath.IsAbs(target):
			target = filepath.Join(filepath.Dir(path), target)
		}
		if err := globals.add(target, absolute, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func readGlobal(path string) (string, bool, error) {
	raw, err := os.ReadFile(path)
	switch {
	case err == nil:
		return document.DecodeUTF8(raw), true, nil
	case errors.Is(err, syscall.ENOENT), errors.Is(err, syscall.ENOTDIR), errors.Is(err, syscall.EISDIR):
		return "", false, nil
	default:
		return "", false, document.FileError("open", err)
	}
}
