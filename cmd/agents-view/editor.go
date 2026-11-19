package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/wlkr-si/agents-view/internal/check"
	"github.com/wlkr-si/agents-view/internal/server"
)

func openEditor(dir string, open bool) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return server.Serve(ctx, dir, findSources, func(address string) {
		fmt.Println(address)
		if !open {
			return
		}
		opener := "xdg-open"
		switch runtime.GOOS {
		case "darwin":
			opener = "open"
		case "windows":
			opener = "explorer.exe"
		}
		go func() {
			if err := exec.CommandContext(ctx, opener, address).Run(); err != nil && ctx.Err() == nil {
				fmt.Fprintf(os.Stderr, "Open %s in your browser.\n", address)
			}
		}()
	})
}

func findSources(_ string, agent string) ([]server.Source, error) {
	globals, err := check.Globals(agent)
	if err != nil {
		return nil, err
	}
	files := make([]server.Source, 0, len(globals))
	for _, file := range globals {
		files = append(files, server.Source{Path: file.Path, Origin: "Global", Text: file.Text, Via: file.Via})
	}
	return files, nil
}
