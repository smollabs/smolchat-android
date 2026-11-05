// Package assets embeds SmolChat's fonts (SIL OFL 1.1; see the OFL-*.txt
// files next to them) and SmolWorks brand artwork.
package assets

import "embed"

//go:embed fonts/*.ttf
var Fonts embed.FS

// Images holds the mascot and logo PNGs.
//
//go:embed images/*.png
var Images embed.FS
