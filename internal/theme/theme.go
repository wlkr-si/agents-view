package theme

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/wlkr-si/agents-view/themes"
)

const (
	system   = "system"
	omarchy  = "omarchy"
	dark     = "norwegian-wood-sunrise"
	light    = "flexoki-light"
	filename = "colors.toml"
)

var (
	slugPattern   = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	slugSeparator = regexp.MustCompile(`[^a-z0-9]+`)
	bundled       = map[string]string{dark: bundledCSS(dark), light: bundledCSS(light)}
	systemCSS     = "@media(prefers-color-scheme:dark){" + bundled[dark] + "}@media(prefers-color-scheme:light){" + bundled[light] + "}"
)

type Store struct {
	dir          string
	omarchyPaths []string
}

type List struct {
	Active  string   `json:"active"`
	Names   []string `json:"names"`
	Problem string   `json:"problem,omitempty"`
}

func Open() (Store, error) {
	home, err := os.UserHomeDir()
	config := os.Getenv("XDG_CONFIG_HOME")
	if !filepath.IsAbs(config) {
		if err != nil {
			return Store{}, err
		}
		config = filepath.Join(home, ".config")
	}
	store := Store{dir: filepath.Join(config, "agents-view")}
	// Omarchy 4 uses the state directory. Older releases used the config directory.
	if home != "" {
		store.omarchyPaths = append(store.omarchyPaths, filepath.Join(home, ".local", "state", "omarchy", "current", "theme", filename))
	}
	store.omarchyPaths = append(store.omarchyPaths, filepath.Join(config, "omarchy", "current", "theme", filename))
	if home != "" && config != filepath.Join(home, ".config") {
		store.omarchyPaths = append(store.omarchyPaths, filepath.Join(home, ".config", "omarchy", "current", "theme", filename))
	}
	return store, nil
}

func bundledCSS(name string) string {
	raw, err := themes.Files.ReadFile(name + "/" + filename)
	if err != nil {
		panic(err)
	}
	css, err := colorsCSS(raw)
	if err != nil {
		panic(fmt.Errorf("%s: %w", name, err))
	}
	return css
}

func (store Store) settings() string {
	return filepath.Join(store.dir, "settings.toml")
}

func (store Store) themeDir() string {
	return filepath.Join(store.dir, "themes")
}

func (store Store) selection() (string, error) {
	raw, err := readFile(store.settings())
	if errors.Is(err, os.ErrNotExist) {
		return system, nil
	}
	if err != nil {
		return system, err
	}
	var name string
	err = assignments(raw, func(key, value string) error {
		if key != "theme" || !validName(value) {
			return errors.New("expected theme = \"<name>\"")
		}
		name = value
		return nil
	})
	if err != nil {
		return system, fmt.Errorf("settings.toml: %w", err)
	}
	return cmp.Or(name, system), nil
}

func validName(name string) bool {
	return len(name) <= 100 && slugPattern.MatchString(name)
}

func reserved(name string) bool {
	return name == system || name == omarchy || bundled[name] != ""
}

func (store Store) css(name string) (string, error) {
	switch name {
	case system:
		return systemCSS, nil
	case omarchy:
		for _, path := range store.omarchyPaths {
			css, err := fileCSS(path)
			if !errors.Is(err, os.ErrNotExist) {
				return css, err
			}
		}
		return "", errors.New("Omarchy's current colors.toml was not found")
	}
	if css := bundled[name]; css != "" {
		return css, nil
	}
	if !validName(name) {
		return "", fmt.Errorf("invalid theme name %q", name)
	}
	css, err := fileCSS(filepath.Join(store.themeDir(), name, filename))
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("theme %q was not found", name)
	}
	return css, err
}

func (store Store) CurrentCSS() string {
	name, _ := store.selection()
	css, _ := store.css(name)
	return cmp.Or(css, systemCSS)
}

func (store Store) List() (List, error) {
	active, err := store.selection()
	list := List{Active: active, Names: []string{system, dark, light}}
	if _, err := store.css(omarchy); err == nil || active == omarchy {
		list.Names = append(list.Names, omarchy)
	}
	if err == nil && active != system {
		_, err = store.css(active)
	}
	if err != nil {
		list.Problem = err.Error()
	}
	entries, err := os.ReadDir(store.themeDir())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return list, err
	}
	for _, entry := range entries {
		name := entry.Name()
		if validName(name) && !reserved(name) {
			info, err := os.Stat(filepath.Join(store.themeDir(), name, filename))
			if err == nil && info.Mode().IsRegular() {
				list.Names = append(list.Names, name)
			}
		}
	}
	return list, nil
}

func (store Store) Use(name string) error {
	if _, err := store.css(name); err != nil {
		return err
	}
	if err := os.MkdirAll(store.dir, 0700); err != nil {
		return err
	}
	return writeFile(store.settings(), []byte(fmt.Sprintf("theme = %q\n", name)))
}

func (store Store) Install(label string, raw []byte) (string, error) {
	if _, err := colorsCSS(raw); err != nil {
		return "", fmt.Errorf("%s: %w", filename, err)
	}
	hash := sha256.Sum256(raw)
	slug := strings.Trim(slugSeparator.ReplaceAllString(strings.ToLower(label), "-"), "-")
	if slug == "" || slug == "colors" || len(slug) > 86 {
		slug = fmt.Sprintf("theme-%x", hash[:6])
	}
	if reserved(slug) {
		slug = fmt.Sprintf("%s-%x", slug, hash[:6])
	}
	if err := os.MkdirAll(store.themeDir(), 0700); err != nil {
		return "", err
	}
	dir := filepath.Join(store.themeDir(), slug)
	path := filepath.Join(dir, filename)
	if err := os.Mkdir(dir, 0700); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return "", err
		}
		if previous, err := readFile(path); err != nil || !bytes.Equal(previous, raw) {
			return "", fmt.Errorf("theme %q already exists", slug)
		}
	} else if err := writeFile(path, raw); err != nil {
		_ = os.Remove(dir)
		return "", err
	}
	return slug, store.Use(slug)
}

func fileCSS(path string) (string, error) {
	raw, err := readFile(path)
	if err != nil {
		return "", err
	}
	css, err := colorsCSS(raw)
	if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	return css, nil
}

func readFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s must be a regular file", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, MaxSize+1))
	if len(raw) > MaxSize {
		return nil, fmt.Errorf("%s exceeds %d KiB", path, MaxSize/1024)
	}
	return raw, err
}

func writeFile(path string, raw []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".theme-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(raw)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
