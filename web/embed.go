package web

import "embed"

// Files contains the browser assets served by the Editor.
//
//go:embed index.html app.js syntax.js style.css theme.js diagram.html diagram.js mermaid.LICENSE
var Files embed.FS

// MermaidGzip holds the compressed renderer without a second uncompressed copy.
//
//go:embed mermaid.min.js.gz
var MermaidGzip string
