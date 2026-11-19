// Command release builds the installers and release assets for a version tag.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const repo = "wlkr-si/agents-view"

var versionTag = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$`)

type target struct{ os, arch string }

var targets = []target{
	{"darwin", "arm64"}, {"darwin", "amd64"},
	{"linux", "arm64"}, {"linux", "amd64"},
	{"windows", "arm64"}, {"windows", "amd64"},
}

func (t target) binary() string {
	if t.os == "windows" {
		return "agents-view.exe"
	}
	return "agents-view"
}

func (t target) archive(version string) string {
	extension := ".tar.gz"
	if t.os == "windows" {
		extension = ".zip"
	}
	return fmt.Sprintf("agents-view_%s_%s_%s%s", version, t.os, t.arch, extension)
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "release:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) < 1 || len(args) > 2 || !versionTag.MatchString(args[0]) || (len(args) == 2 && args[1] != "--publish") {
		return errors.New("usage: go run ./tools/release vMAJOR.MINOR.PATCH[-PRERELEASE] [--publish]")
	}
	tag := args[0]
	publishing := len(args) == 2
	if publishing {
		if err := verifyTag(tag); err != nil {
			return err
		}
	}

	if err := os.MkdirAll("dist", 0o755); err != nil {
		return err
	}
	work, err := os.MkdirTemp("dist", ".release-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)

	version := strings.TrimPrefix(tag, "v")
	sums := make(map[string]string)
	var assets []string
	for _, t := range targets {
		name := t.archive(version)
		size, err := buildArchive(work, tag, t, name)
		if err != nil {
			return fmt.Errorf("%s/%s: %w", t.os, t.arch, err)
		}
		archive, err := os.ReadFile(filepath.Join(work, name))
		if err != nil {
			return err
		}
		sums[name] = fmt.Sprintf("%x", sha256.Sum256(archive))
		assets = append(assets, name)
		fmt.Printf("%s  %d bytes (binary %d bytes)\n", name, len(archive), size)
	}

	var checksums strings.Builder
	for _, name := range assets {
		fmt.Fprintf(&checksums, "%s  %s\n", sums[name], name)
	}
	if err := os.WriteFile(filepath.Join(work, "checksums.txt"), []byte(checksums.String()), 0o644); err != nil {
		return err
	}
	assets = append(assets, "checksums.txt")
	for _, name := range assets {
		if err := os.Rename(filepath.Join(work, name), filepath.Join("dist", name)); err != nil {
			return err
		}
	}
	fmt.Printf("\ndist/checksums.txt\n%s\n", checksums.String())

	npmDir, err := stageNPM(work, version)
	if err != nil {
		return err
	}
	if err := renderFormula(tag, sums); err != nil {
		return err
	}
	fmt.Println("Homebrew formula: dist/Formula/agents-view.rb")
	fmt.Println("npm package:", npmDir)
	if !publishing {
		fmt.Println("Dry run complete. Nothing published.")
		return nil
	}
	return publish(work, tag, assets, sums, npmDir)
}

func stageNPM(work, version string) (string, error) {
	stage := filepath.Join(work, "npm")
	if err := os.MkdirAll(filepath.Join(stage, "bin"), 0o755); err != nil {
		return "", err
	}
	for _, name := range []string{"README.md", "LICENSE", "npm/bin/agents-view.js"} {
		data, err := os.ReadFile(name)
		if err != nil {
			return "", err
		}
		mode := os.FileMode(0o644)
		if strings.HasPrefix(name, "npm/bin/") {
			mode = 0o755
		}
		if err := os.WriteFile(filepath.Join(stage, strings.TrimPrefix(name, "npm/")), data, mode); err != nil {
			return "", err
		}
	}
	data, err := os.ReadFile("npm/package.json")
	if err != nil {
		return "", err
	}
	var manifest map[string]json.RawMessage
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "", err
	}
	manifest["version"], err = json.Marshal(version)
	if err != nil {
		return "", err
	}
	data, err = json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(stage, "package.json"), append(data, '\n'), 0o644); err != nil {
		return "", err
	}
	dir := filepath.Join("dist", "npm")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dest := filepath.Join(dir, version)
	// Replace only the generated package for this version.
	if err := os.RemoveAll(dest); err != nil {
		return "", err
	}
	return dest, os.Rename(stage, dest)
}

func command(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}
