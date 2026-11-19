package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type archiveFile struct {
	name string
	mode os.FileMode
	data []byte
}

func buildArchive(work, tag string, t target, name string) (int, error) {
	binary := filepath.Join(work, t.binary())
	build := exec.Command("go", "build", "-trimpath", "-buildvcs=false", "-ldflags=-s -w -X main.version="+tag, "-o", binary, "./cmd/agents-view")
	build.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+t.os, "GOARCH="+t.arch)
	build.Stdout, build.Stderr = os.Stdout, os.Stderr
	if err := build.Run(); err != nil {
		return 0, err
	}

	// Keep the binary first so the npm launcher can read just that entry.
	names := []string{t.binary(), "LICENSE", "README.md"}
	files := make([]archiveFile, 0, len(names))
	for i, name := range names {
		path, mode := name, os.FileMode(0o644)
		if i == 0 {
			path, mode = binary, 0o755
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return 0, err
		}
		files = append(files, archiveFile{name, mode, data})
	}
	size := len(files[0].data)
	if size >= 10_000_000 {
		return 0, fmt.Errorf("binary is %d bytes, above the release limit of 10 MB", size)
	}
	path := filepath.Join(work, name)
	out, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	defer out.Close()
	if t.os == "windows" {
		err = writeZip(out, files)
	} else {
		err = writeTarGzip(out, files)
	}
	if err != nil {
		return 0, err
	}
	return size, out.Close()
}

func writeTarGzip(out *os.File, files []archiveFile) error {
	gz := gzip.NewWriter(out)
	tw := tar.NewWriter(gz)
	for _, file := range files {
		header := &tar.Header{Name: file.name, Mode: int64(file.mode), Size: int64(len(file.data)), Format: tar.FormatUSTAR}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if _, err := tw.Write(file.data); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

func writeZip(out *os.File, files []archiveFile) error {
	zw := zip.NewWriter(out)
	for _, file := range files {
		header := &zip.FileHeader{Name: file.name, Method: zip.Deflate}
		header.SetMode(file.mode)
		entry, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		if _, err := entry.Write(file.data); err != nil {
			return err
		}
	}
	return zw.Close()
}
