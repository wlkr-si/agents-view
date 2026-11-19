package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/wlkr-si/agents-view/internal/server"
	"github.com/wlkr-si/agents-view/internal/sources"
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

func findSources(dir, agent string) ([]server.Source, error) {
	discovered, err := sources.DiscoverLocal(dir, agent)
	if err != nil {
		return nil, err
	}
	files := make([]server.Source, 0, len(discovered))
	for _, file := range discovered {
		files = append(files, server.Source{Path: file.Path, Origin: string(file.Origin), Text: file.Text, Via: file.Via, OnDemand: file.OnDemand})
	}
	return files, nil
}
