package web

import "embed"

// Files contains the browser assets served by the Editor.
//
//go:embed index.html app.js syntax.js style.css theme.js
var Files embed.FS
