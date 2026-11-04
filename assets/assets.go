// Package assets embeds the fonts SmolChat ships with (SIL OFL 1.1; see the
// OFL-*.txt files next to them).
package assets

import "embed"

//go:embed fonts/*.ttf
var Fonts embed.FS
