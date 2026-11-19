// Command vendor-mermaid refreshes the pinned browser bundle and MIT license.
package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const upstream = "https://cdn.jsdelivr.net/npm/mermaid@11.12.2/"

func main() {
	client := &http.Client{Timeout: 30 * time.Second}
	bundle, err := download(client, "dist/mermaid.min.js", "d0830a6c05546e9edb8fe20a8f545f3e0dc7c4c3134d584bad9c13a99d7a71e0")
	must(err)
	license, err := download(client, "LICENSE", "ec9fb67dcb25eccc416ed56e1aab819222c805a2a4bfe4cb19e7556bf2ffde80")
	must(err)

	var packed bytes.Buffer
	writer, err := gzip.NewWriterLevel(&packed, gzip.BestCompression)
	must(err)
	_, err = writer.Write(bundle)
	must(err)
	must(writer.Close())
	must(os.WriteFile("web/mermaid.min.js.gz", packed.Bytes(), 0o644))
	must(os.WriteFile("web/mermaid.LICENSE", license, 0o644))
	fmt.Printf("Mermaid 11.12.2: verified bundle and license, %d gzip bytes\n", packed.Len())
}

func download(client *http.Client, path, digest string) ([]byte, error) {
	response, err := client.Get(upstream + path)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", path, response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if actual := fmt.Sprintf("%x", sha256.Sum256(data)); actual != digest {
		return nil, fmt.Errorf("%s: SHA-256 is %s, expected %s", path, actual, digest)
	}
	return data, nil
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "vendor-mermaid:", err)
		os.Exit(1)
	}
}
