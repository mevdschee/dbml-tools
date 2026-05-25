package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"runtime"
)

// Build-time variables. Set via -ldflags "-X main.version=… -X main.commit=… -X main.buildDate=…".
// The defaults are used when running from `go run`.
var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
)

// doVersion prints version metadata. With --sha256 it also prints the
// SHA-256 of the running executable, so a downloaded binary can be verified
// against the SHA256SUMS file shipped alongside each release.
func doVersion(args []string) {
	verifyHash := false
	for _, a := range args {
		switch a {
		case "--sha256", "--hash":
			verifyHash = true
		case "-h", "--help":
			fmt.Println("Usage: dbml-tools version [--sha256]")
			return
		}
	}

	fmt.Printf("dbml-tools %s\n", version)
	fmt.Printf("  commit:     %s\n", commit)
	fmt.Printf("  built:      %s\n", buildDate)
	fmt.Printf("  go:         %s\n", runtime.Version())
	fmt.Printf("  platform:   %s/%s\n", runtime.GOOS, runtime.GOARCH)

	if verifyHash {
		h, err := selfSHA256()
		if err != nil {
			fmt.Fprintf(os.Stderr, "  sha256:     error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("  sha256:     %s\n", h)
	}
}

// selfSHA256 returns the hex-encoded SHA-256 of the currently running binary.
func selfSHA256() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	f, err := os.Open(exe)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
