package themes

import "embed"

// Files contains the bundled Omarchy themes.
//
//go:embed */colors.toml */LICENSE
var Files embed.FS
