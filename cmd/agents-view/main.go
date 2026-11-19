package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wlkr-si/agents-view/internal/check"
	"github.com/wlkr-si/agents-view/internal/document"
	"github.com/wlkr-si/agents-view/internal/sources"
)

var version = "dev"

const usage = `agents-view [--no-open]
agents-view --version
agents-view sections
agents-view read [section]
agents-view write <section> < body.md
agents-view check
agents-view sources [--agent claude|codex|opencode|gemini|cursor|copilot]
agents-view theme [use <name|system|omarchy> | add <git-url>]`

func main() {
	code, err := run(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		code = 1
	}
	os.Exit(code)
}

func run(args []string) (int, error) {
	if len(args) == 1 && args[0] == "--version" {
		fmt.Println("agents-view", version)
		return 0, nil
	}
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Println(usage)
		return 0, nil
	}
	command := "view"
	if len(args) > 0 && args[0] != "--no-open" {
		command, args = args[0], args[1:]
	}
	if command == "theme" {
		return 0, runTheme(args)
	}
	switch command {
	case "view":
		if len(args) != 0 && (len(args) != 1 || args[0] != "--no-open") {
			return 1, errors.New(usage)
		}
	case "sections", "check":
		if len(args) != 0 {
			return 1, errors.New(usage)
		}
	case "read":
		if len(args) > 1 {
			return 1, errors.New(usage)
		}
	case "write":
		if len(args) != 1 {
			return 1, errors.New(usage)
		}
	case "sources":
		if len(args) != 0 && (len(args) != 2 || args[0] != "--agent") {
			return 1, errors.New(usage)
		}
	default:
		return 1, errors.New(usage)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return 1, err
	}
	// Node resolves the physical working directory, even when PWD is a symlink.
	cwd, err = filepath.EvalSymlinks(cwd)
	if err != nil {
		return 1, err
	}
	if command == "view" {
		return 0, openEditor(cwd, len(args) == 0)
	}
	if command == "sources" {
		agent := "claude"
		if len(args) == 2 {
			agent = args[1]
		}
		files, err := sources.Discover(cwd, agent)
		if err != nil {
			return 1, err
		}
		fmt.Println(agent)
		for _, file := range files {
			fmt.Printf("%s\t~%d tokens\n", file.Path, file.Tokens)
		}
		return 0, nil
	}
	file := filepath.Join(cwd, "AGENTS.md")
	if command == "write" {
		body, err := document.ReadBody()
		if err != nil {
			return 1, err
		}
		return 0, document.Update(file, args[0], body, nil, false)
	}
	text, err := document.ReadFile(file)
	if err != nil {
		return 1, err
	}
	switch command {
	case "sections":
		for _, section := range document.Parse(text).Sections {
			fmt.Printf("%s\t%s\t~%d tokens\n", section.DisplayName(), section.Type, section.Tokens)
		}
	case "read":
		if len(args) > 0 {
			section, err := document.Parse(text).FindSection(args[0])
			if err != nil {
				return 1, err
			}
			if section == nil {
				return 1, fmt.Errorf("Unknown section: %s", args[0])
			}
			text = text[section.Start-len(section.Heading) : section.End]
		}
		fmt.Print(text)
	case "check":
		result, err := check.Check(file, text)
		if err != nil {
			return 1, err
		}
		for _, warning := range result.Warnings {
			fmt.Fprintln(os.Stderr, "Warning: "+warning)
		}
		for _, problem := range result.Errors {
			fmt.Fprintln(os.Stderr, "Error: "+problem)
		}
		if len(result.Errors) > 0 {
			return 1, nil
		}
		fmt.Printf("OK · ~%d / 1,000 tokens · Pages and Overrides valid\n", result.Tokens)
	}
	return 0, nil
}
