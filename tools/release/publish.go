package main

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
)

const tap = "wlkr-si/homebrew-tap"
const formulaPath = "Formula/agents-view.rb"

//go:embed agents-view.rb.tmpl
var formulaTemplate string

func renderFormula(tag string, sums map[string]string) error {
	type asset struct{ Name, SHA string }
	data := struct {
		Base, Version                                string
		DarwinARM, DarwinIntel, LinuxARM, LinuxIntel asset
	}{
		Base:    "https://github.com/" + repo + "/releases/download/" + tag,
		Version: strings.TrimPrefix(tag, "v"),
	}
	for _, item := range []struct {
		target target
		asset  *asset
	}{
		{target{"darwin", "arm64"}, &data.DarwinARM},
		{target{"darwin", "amd64"}, &data.DarwinIntel},
		{target{"linux", "arm64"}, &data.LinuxARM},
		{target{"linux", "amd64"}, &data.LinuxIntel},
	} {
		entry := item.asset
		entry.Name = item.target.archive(data.Version)
		entry.SHA = sums[entry.Name]
		if !sha256Hex.MatchString(entry.SHA) {
			return fmt.Errorf("missing or invalid checksum for %s", entry.Name)
		}
	}
	tmpl, err := template.New("formula").Parse(formulaTemplate)
	if err != nil {
		return err
	}
	var formula strings.Builder
	if err := tmpl.Execute(&formula, data); err != nil {
		return err
	}
	path := filepath.Join("dist", formulaPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(formula.String()), 0o644)
}

var sha256Hex = regexp.MustCompile(`^[a-f0-9]{64}$`)

func verifyTag(tag string) error {
	status, err := exec.Command("git", "status", "--porcelain").Output()
	if err != nil || len(status) != 0 {
		return fmt.Errorf("publishing requires a clean checkout of %s", tag)
	}
	head, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return err
	}
	remote, err := exec.Command("gh", "api", "repos/"+repo+"/commits/"+tag, "--jq", ".sha").Output()
	if err != nil {
		return fmt.Errorf("read pushed tag %s: %w", tag, err)
	}
	if strings.TrimSpace(string(head)) != strings.TrimSpace(string(remote)) {
		return fmt.Errorf("check out the pushed tag %s before publishing", tag)
	}
	return nil
}

func publish(work, tag string, assets []string, sums map[string]string, npmDir string) error {
	ci := os.Getenv("GITHUB_ACTIONS") == "true"
	prerelease := strings.Contains(tag, "-")
	// Tag CI can publish the release before the local run publishes the installers.
	release, err := exec.Command("gh", "release", "view", tag, "--repo", repo, "--json", "isDraft", "--jq", ".isDraft").Output()
	if err == nil && strings.TrimSpace(string(release)) == "false" {
		if !ci {
			if err := command("", "gh", "release", "download", tag, "--repo", repo, "--pattern", "checksums.txt", "--dir", work, "--clobber"); err != nil {
				return err
			}
			data, err := os.ReadFile(filepath.Join(work, "checksums.txt"))
			if err != nil {
				return err
			}
			published := make(map[string]string)
			for _, line := range strings.Split(string(data), "\n") {
				fields := strings.Fields(line)
				if len(fields) != 2 {
					continue
				}
				hash, name := fields[0], fields[1]
				if _, ok := sums[name]; ok && sha256Hex.MatchString(hash) {
					if _, duplicate := published[name]; duplicate {
						return fmt.Errorf("duplicate published checksum for %s", name)
					}
					published[name] = hash
				}
			}
			if len(published) != len(sums) {
				return fmt.Errorf("published checksums for %s do not cover all six assets", tag)
			}
			if err := renderFormula(tag, published); err != nil {
				return err
			}
		}
		fmt.Println("GitHub release already published:", tag)
	} else {
		args := []string{"release", "create", tag, "--repo", repo, "--verify-tag", "--generate-notes"}
		if prerelease {
			args = append(args, "--prerelease")
		}
		for _, name := range assets {
			args = append(args, filepath.Join("dist", name))
		}
		if err := command("", "gh", args...); err != nil {
			return err
		}
	}

	if ci {
		fmt.Println("GitHub Actions: the next step publishes npm through trusted publishing; the Homebrew tap follows the release on its own.")
		return nil
	}
	if err := publishTap(work, tag); err != nil {
		return err
	}
	args := []string{"publish"}
	if prerelease {
		args = append(args, "--tag", "next")
	}
	if exec.Command("npm", "whoami").Run() != nil {
		fmt.Println("After signing in to npm, run this from the Agents View repo root:")
		fmt.Println("npm " + strings.Join(args, " ") + " ./" + filepath.ToSlash(npmDir))
		return nil
	}
	return command(npmDir, "npm", args...)
}

func publishTap(work, tag string) error {
	dir := filepath.Join(work, "tap")
	if err := command("", "gh", "repo", "clone", tap, dir, "--", "--depth", "1"); err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join("dist", formulaPath))
	if err != nil {
		return err
	}
	path := filepath.Join(dir, formulaPath)
	if current, err := os.ReadFile(path); err == nil && string(current) == string(data) {
		fmt.Println("Homebrew formula already up to date:", tag)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	if err := command(dir, "git", "add", formulaPath); err != nil {
		return err
	}
	if err := command(dir, "git", "commit", "-m", "Agents View "+tag); err != nil {
		return err
	}
	return command(dir, "git", "push")
}
