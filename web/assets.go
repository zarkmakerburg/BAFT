// Package webui bundles the command center in the BCC binary.
package webui

import (
	"embed"
	"encoding/json"
	"html"
	"strings"
)

// PageFile is the Command Center page template; it is served only through Page.
const PageFile = "bcc-app.html"

// FS contains only public UI assets. No credentials or state are embedded.
//
//go:embed bcc-app.html bcc-app.css bcc-app.js bcc-map.js bcc-operations-v3.js bcc-earth-v3.js bcc-welcome-globe-v3.js assets/*
var FS embed.FS

type Config struct {
	OperationsHTML string          `json:"operationsHTML,omitempty"`
	OperationsJS   string          `json:"operationsJS,omitempty"`
	CSRF           string          `json:"csrf,omitempty"`
	CSRFHeader     string          `json:"csrfHeader,omitempty"`
	Mode           string          `json:"mode"`
	Base           string          `json:"base"`
	LoginAction    string          `json:"loginAction"`
	Error          string          `json:"error,omitempty"`
	Nonce          string          `json:"nonce,omitempty"` // sent to client so JS can stamp nonces on dynamic scripts
	// Wallets, when non-nil, overrides the support wallet list in the UI.
	// Omit to show no wallets; supply a JSON array of wallet objects to show them.
	Wallets        json.RawMessage `json:"wallets,omitempty"`
}

// Page renders the Command Center for one request. Assets are served under
// <base>v3/, whichever path the page itself is served at.
func Page(config Config) string {
	raw, _ := FS.ReadFile(PageFile)
	data, _ := json.Marshal(config) // Escapes HTML-sensitive characters.
	nonceAttr := ""
	if config.Nonce != "" {
		nonceAttr = ` nonce="` + html.EscapeString(config.Nonce) + `"`
	}
	// Inject per-request config as a nonced inline script.
	configScript := "<script" + nonceAttr + ">window.BAFT_UI=" + string(data) + ";</script>"
	page := strings.Replace(string(raw), "<head>", "<head>"+configScript, 1)
	// Stamp the nonce onto the pre-paint inline script already in bcc-app.html.
	page = strings.Replace(page, `<script id="prepaint">`, `<script id="prepaint"`+nonceAttr+`>`, 1)
	if config.Base != "" {
		base := html.EscapeString(config.Base)
		page = strings.NewReplacer(`="./`, `="`+base+`v3/`, `action="login"`, `action="`+html.EscapeString(config.LoginAction)+`"`, `href="login"`, `href="`+base+`login"`).Replace(page)
	}
	return page
}
