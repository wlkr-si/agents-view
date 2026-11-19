package main

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/wlkr-si/agents-view/internal/theme"
)

func runTheme(args []string) error {
	store, err := theme.Open()
	if err != nil {
		return err
	}
	switch {
	case len(args) == 0:
		list, err := store.List()
		if err != nil {
			return err
		}
		for _, name := range list.Names {
			mark := " "
			if list.Active == name {
				mark = "*"
			}
			fmt.Printf("%s %s\n", mark, name)
		}
		if list.Problem != "" {
			fmt.Fprintln(os.Stderr, list.Problem)
		}
	case len(args) == 2 && args[0] == "use":
		if err := store.Use(args[1]); err != nil {
			return err
		}
		fmt.Printf("Using %s\n", args[1])
	case len(args) == 2 && args[0] == "add":
		name, err := addTheme(store, args[1])
		if err != nil {
			return err
		}
		fmt.Printf("Installed and activated %s\n", name)
	default:
		return fmt.Errorf("%s", usage)
	}
	return nil
}

func addTheme(store theme.Store, url string) (string, error) {
	dir, err := os.MkdirTemp("", "agents-view-theme-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	repo := filepath.Join(dir, "theme")
	clone := exec.Command("git", "-c", "protocol.ext.allow=never", "-c", "protocol.file.allow=never",
		"clone", "--depth", "1", "--no-checkout", "--", url, repo)
	clone.Stdout, clone.Stderr = os.Stdout, os.Stderr
	if err := clone.Run(); err != nil {
		return "", fmt.Errorf("clone theme: %w", err)
	}

	// Read the root blob directly so theme files and symlinks are never checked out.
	const blob = "HEAD:colors.toml"
	size, err := exec.Command("git", "-C", repo, "cat-file", "-s", blob).Output()
	if err != nil {
		return "", fmt.Errorf("theme repository must contain a root colors.toml")
	}
	if n, err := strconv.Atoi(strings.TrimSpace(string(size))); err != nil || n > theme.MaxSize {
		return "", fmt.Errorf("colors.toml exceeds %d KiB", theme.MaxSize/1024)
	}
	raw, err := exec.Command("git", "-C", repo, "cat-file", "blob", blob).Output()
	if err != nil {
		return "", fmt.Errorf("read colors.toml: %w", err)
	}
	location, _, _ := strings.Cut(url, "?")
	location, _, _ = strings.Cut(location, "#")
	name := strings.TrimSuffix(path.Base(location), ".git")
	return store.Install(name, raw)
}
