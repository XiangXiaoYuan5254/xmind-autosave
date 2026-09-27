//go:build !windows

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "XMind Auto Save for Windows only runs on Windows; build it with GOOS=windows GOARCH=amd64.")
	os.Exit(1)
}
