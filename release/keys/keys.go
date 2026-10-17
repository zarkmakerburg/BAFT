// Package keys carries the committed public half of the offline release root
// key. The CLI trusts exactly the file install.sh pins (tests/docs keeps the
// two equal), so there is no second place to rotate.
package keys

import (
	"crypto/ed25519"
	_ "embed"
	"strings"

	"github.com/zarkmakerburg/baft/internal/release"
)

//go:embed root.pub
var rootPub string

// Root returns the pinned release root public key.
func Root() (ed25519.PublicKey, error) {
	return release.DecodePublic(strings.TrimSpace(rootPub))
}
