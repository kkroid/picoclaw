//go:build windows

package backend

import _ "embed"

//go:embed icon.ico
var iconICO []byte

func getIcon() []byte {
	return iconICO
}
