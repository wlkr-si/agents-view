package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wlkr-si/agents-view/internal/check"
	"github.com/wlkr-si/agents-view/internal/document"
)

const usage = `agents-view sections
agents-view read [section]
agents-view write <section> < body.md
agents-view check
agents-view sources [--agent claude|codex]
agents-view view [--no-open]`

func main() {
	code, err := run(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		code = 1
	}
	os.Exit(code)
}

func run(args []string) (int, error) {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Println(usage)
		return 0, nil
	}
	command, args := args[0], args[1:]
	switch command {
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
