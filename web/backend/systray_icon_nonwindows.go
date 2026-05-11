//go:build !windows && ((!darwin && !freebsd) || cgo)

package backend

import _ "embed"

//go:embed icon.png
var iconPNG []byte

func getIcon() []byte {
	return iconPNG
}
