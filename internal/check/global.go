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

func globalFiles() ([]string, error) {
	home, _ := os.UserHomeDir()
	if home == "" {
		if current, err := user.Current(); err == nil {
			home = current.HomeDir
		}
	}
	claude := os.Getenv("CLAUDE_CONFIG_DIR")
	if claude == "" {
		claude = filepath.Join(home, ".claude")
	}
	globals := claudeGlobals{seen: make(map[string]bool), home: home}
	if err := globals.add(filepath.Join(claude, "CLAUDE.md"), 0); err != nil {
		return nil, err
	}

	codex := os.Getenv("CODEX_HOME")
	if codex == "" {
		codex = filepath.Join(home, ".codex")
	}
	for _, name := range []string{"AGENTS.override.md", "AGENTS.md"} {
		text, _, err := readGlobal(filepath.Join(codex, name))
		if err != nil {
			return nil, err
		}
		if document.TrimSpace(text) != "" {
			globals.files = append(globals.files, text)
			break
		}
	}
	return globals.files, nil
}

type claudeGlobals struct {
	files []string
	seen  map[string]bool
	home  string
}

func (globals *claudeGlobals) add(path string, depth int) error {
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
	globals.files = append(globals.files, text)
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
		if err := globals.add(target, depth+1); err != nil {
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
