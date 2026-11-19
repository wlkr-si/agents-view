package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/wlkr-si/agents-view/internal/document"
)

type diagramEdit struct {
	Name, Change string
}

func (editor *editor) changeDiagram(edit diagramEdit) error {
	change := document.TrimSpace(edit.Change)
	if change == "" {
		return errors.New("Describe the diagram change.")
	}
	section, err := editor.findSection(edit.Name)
	if err != nil {
		return err
	}
	if section == nil || section.Type != document.Diagram {
		return errors.New("Choose an existing diagram.")
	}

	argv := []string{"codex", "exec", "-s", "workspace-write", "-"}
	if configured, ok := os.LookupEnv("AGENTS_VIEW_AGENT_COMMAND"); ok {
		argv = nil
		if err := json.Unmarshal([]byte(configured), &argv); err != nil ||
			len(argv) == 0 || slices.Contains(argv, "") {
			return errors.New("AGENTS_VIEW_AGENT_COMMAND must be a JSON array of command arguments.")
		}
	}
	cli, err := os.Executable()
	if err != nil {
		return err
	}
	name, _ := json.Marshal(edit.Name)
	prompt := fmt.Sprintf(
		"Update only the %s diagram section in AGENTS.md in the current working directory.\n"+
			"Read that section, keep the Mermaid fence and any accompanying agent-readable text, and preserve all other sections.\n"+
			"Feed the complete replacement body on stdin to agents-view write %s. If the bin is unavailable, use %s write %s.\n"+
			"Requested change: %s",
		name, shellQuote(edit.Name), shellQuote(cli), shellQuote(edit.Name), change,
	)
	command := exec.CommandContext(editor.ctx, argv[0], argv[1:]...)
	command.Dir = editor.dir
	command.Stdin = strings.NewReader(prompt)
	stopJobWithServer(command)
	return editor.events.startJob(command)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func (events *events) startJob(command *exec.Cmd) error {
	events.mu.Lock()
	defer events.mu.Unlock()
	if events.running != nil {
		return errors.New("A diagram edit is already running.")
	}
	events.job = "Diagram edit running…"
	events.broadcastLocked(event{"job", events.job})
	done := make(chan struct{})
	events.running = done
	go func() {
		err := command.Run()
		status := "Diagram edit finished."
		var exit *exec.ExitError
		switch {
		case errors.As(err, &exit):
			code := "signal"
			if exit.ExitCode() >= 0 {
				code = fmt.Sprint(exit.ExitCode())
			}
			status = "Diagram edit exited with status " + code + "."
		case err != nil:
			status = err.Error()
		}
		events.mu.Lock()
		defer events.mu.Unlock()
		events.job = status
		events.running = nil
		events.broadcastLocked(event{"job", status})
		close(done)
	}()
	return nil
}

func (events *events) waitForJob() {
	events.mu.Lock()
	done := events.running
	events.mu.Unlock()
	if done != nil {
		<-done
	}
}
