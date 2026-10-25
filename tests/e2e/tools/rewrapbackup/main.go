// rewrapbackup re-encrypts a BAFT BCC encrypted backup under a different
// (older) backup schema number so the e2e can prove that the current restore
// still reads legacy backups. LAB tool: it needs the backup key and is never
// shipped. Only the header's schema_version changes; the payload is kept.
//
//	rewrapbackup -in v4.baftbak -key-file key -schema 3 -out v3.baftbak
package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/zarkmakerburg/baft/internal/bcc"
)

type envelope struct {
	Header     bcc.BackupHeader `json:"header"`
	Nonce      string           `json:"nonce"`
	Ciphertext string           `json:"ciphertext"`
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "rewrapbackup:", err)
	os.Exit(1)
}

func main() {
	in := flag.String("in", "", "input backup")
	keyFile := flag.String("key-file", "", "file with the base64 32-byte backup key")
	schema := flag.Int("schema", 3, "schema_version to write")
	out := flag.String("out", "", "output backup")
	flag.Parse()
	if *in == "" || *keyFile == "" || *out == "" {
		die(fmt.Errorf("-in, -key-file and -out are required"))
	}
	raw, err := os.ReadFile(*keyFile)
	if err != nil {
		die(err)
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(key) != 32 {
		die(fmt.Errorf("bad key"))
	}
	data, err := os.ReadFile(*in)
	if err != nil {
		die(err)
	}
	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		die(err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		die(err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		die(err)
	}
	aad, err := json.Marshal(env.Header)
	if err != nil {
		die(err)
	}
	nonce, err := base64.StdEncoding.DecodeString(env.Nonce)
	if err != nil {
		die(err)
	}
	ct, err := base64.StdEncoding.DecodeString(env.Ciphertext)
	if err != nil {
		die(err)
	}
	plain, err := aead.Open(nil, nonce, ct, aad)
	if err != nil {
		die(fmt.Errorf("input backup does not authenticate: %w", err))
	}
	env.Header.SchemaVersion = *schema
	aad, err = json.Marshal(env.Header)
	if err != nil {
		die(err)
	}
	nn := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nn); err != nil {
		die(err)
	}
	env.Nonce = base64.StdEncoding.EncodeToString(nn)
	env.Ciphertext = base64.StdEncoding.EncodeToString(aead.Seal(nil, nn, plain, aad))
	b, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		die(err)
	}
	if err := os.WriteFile(*out, b, 0o600); err != nil {
		die(err)
	}
}
