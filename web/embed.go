package web

import "embed"

// Files contains the browser assets served by the Editor.
//
//go:embed index.html app.js style.css diagram.html diagram.js
var Files embed.FS
