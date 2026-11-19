// Command budget enforces the published speed promise: it builds the release
// binary and fails when the binary, the Editor assets or the startup time grow
// past their limits, or when the code gains an outbound network client.
package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

const (
	binaryLimit  = 10 << 20
	editorLimit  = 20 << 10
	startupLimit = 50 * time.Millisecond
	startupRuns  = 5
)

var outbound = regexp.MustCompile(`http\.(Get|Post|Head|PostForm|NewRequest)\(|&http\.Client\{|http\.DefaultClient|net\.Dial\(`)

func main() {
	failed := false
	report := func(name, value, limit string, ok bool) {
		mark := "ok"
		if !ok {
			mark = "OVER"
			failed = true
		}
		fmt.Printf("%-24s %12s  limit %-8s %s\n", name, value, limit, mark)
	}

	work, err := os.MkdirTemp("", "agents-view-budget-")
	must(err)
	defer os.RemoveAll(work)

	binary := filepath.Join(work, "agents-view")
	build := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", binary, "./cmd/agents-view")
	build.Stdout, build.Stderr = os.Stdout, os.Stderr
	must(build.Run())
	info, err := os.Stat(binary)
	must(err)
	report("binary", kb(info.Size()), "10 MB", info.Size() <= binaryLimit)

	editor := editorBytes("web")
	report("Editor assets (gzip)", kb(editor), "20 KB", editor <= editorLimit)

	startup := medianStartup(binary, work)
	report("startup (median of 5)", fmt.Sprintf("%.1f ms", float64(startup.Microseconds())/1000), "50 ms", startup <= startupLimit)

	clients := outboundClients(".")
	report("outbound network code", fmt.Sprint(len(clients)), "0", len(clients) == 0)
	for _, place := range clients {
		fmt.Println("  ", place)
	}

	if failed {
		os.Exit(1)
	}
}

// editorBytes sums the gzipped size of the Editor's own files, leaving out the
// diagram page and the Mermaid renderer, which load only when a diagram shows.
func editorBytes(dir string) int64 {
	var total int64
	entries, err := os.ReadDir(dir)
	must(err)
	for _, entry := range entries {
		name := entry.Name()
		ext := filepath.Ext(name)
		if entry.IsDir() || !slices.Contains([]string{".html", ".css", ".js"}, ext) || strings.HasPrefix(name, "diagram") || strings.HasPrefix(name, "mermaid") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		must(err)
		var packed bytes.Buffer
		writer, _ := gzip.NewWriterLevel(&packed, gzip.BestCompression)
		writer.Write(data)
		writer.Close()
		fmt.Printf("  %-20s %8s gzip\n", name, kb(int64(packed.Len())))
		total += int64(packed.Len())
	}
	return total
}

// medianStartup times launch to the first answered request, in a throwaway
// project so the measurement never touches a real AGENTS.md.
func medianStartup(binary, work string) time.Duration {
	project := filepath.Join(work, "project")
	must(os.MkdirAll(project, 0o755))
	must(os.WriteFile(filepath.Join(project, "AGENTS.md"), []byte("<!-- agents-view v0 -->\n# budget\n\n## Rules\n\n- Measure startup.\n"), 0o644))
	must(exec.Command("git", "init", "-q", project).Run())

	runs := make([]time.Duration, 0, startupRuns)
	for range startupRuns + 1 {
		command := exec.Command(binary, "--no-open")
		command.Dir = project
		command.Env = append(os.Environ(), "HOME="+work)
		output, err := command.StdoutPipe()
		must(err)
		start := time.Now()
		must(command.Start())
		address := ""
		lines := bufio.NewScanner(output)
		for address == "" && lines.Scan() {
			if index := strings.Index(lines.Text(), "http://"); index >= 0 {
				address = strings.Fields(lines.Text()[index:])[0]
			}
		}
		for {
			response, err := http.Get(address + "/")
			if err == nil {
				response.Body.Close()
				break
			}
			if time.Since(start) > 5*time.Second {
				must(fmt.Errorf("the Editor did not answer at %q", address))
			}
		}
		runs = append(runs, time.Since(start))
		command.Process.Kill()
		command.Wait()
	}
	runs = runs[1:] // the first launch pays the operating system's first-run checks
	slices.Sort(runs)
	return runs[len(runs)/2]
}

// outboundClients finds code that could reach the network on its own.
func outboundClients(root string) []string {
	var found []string
	filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && (strings.HasPrefix(entry.Name(), ".") || entry.Name() == "node_modules" || path == filepath.Join("tools", "budget")) {
			return filepath.SkipDir
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for number, line := range strings.Split(string(data), "\n") {
			if outbound.MatchString(line) {
				found = append(found, fmt.Sprintf("%s:%d", path, number+1))
			}
		}
		return nil
	})
	return found
}

func kb(size int64) string {
	if size >= 1<<20 {
		return fmt.Sprintf("%.2f MB", float64(size)/(1<<20))
	}
	return fmt.Sprintf("%.2f KB", float64(size)/(1<<10))
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "budget:", err)
		os.Exit(2)
	}
}
