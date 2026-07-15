// Command sign produces a detached ed25519 signature over a file's bytes.
// It reads the base64-encoded 64-byte private key from DETRITUS_SIGNING_KEY,
// signs the file named by its single argument, and writes "<file>.sig"
// containing the base64 signature plus a trailing newline. It is invoked by
// goreleaser to sign checksums.txt at release time.
package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"os"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "sign:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 2 {
		return fmt.Errorf("usage: sign <file>")
	}
	file := os.Args[1]

	keyB64 := os.Getenv("DETRITUS_SIGNING_KEY")
	if keyB64 == "" {
		return fmt.Errorf("DETRITUS_SIGNING_KEY not set")
	}
	key, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return fmt.Errorf("decode DETRITUS_SIGNING_KEY: %w", err)
	}
	if len(key) != ed25519.PrivateKeySize {
		return fmt.Errorf("private key is %d bytes, want %d", len(key), ed25519.PrivateKeySize)
	}

	data, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("read %s: %w", file, err)
	}

	sig := ed25519.Sign(ed25519.PrivateKey(key), data)
	out := base64.StdEncoding.EncodeToString(sig) + "\n"
	if err := os.WriteFile(file+".sig", []byte(out), 0o644); err != nil {
		return fmt.Errorf("write %s.sig: %w", file, err)
	}
	return nil
}
