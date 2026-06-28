package output

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// Fatal prints a formatted message to stderr and exits with code 1.
func Fatal(format string, v ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", v...)
	os.Exit(1)
}

// PrintBanner renders the tunnel activation banner.
func PrintBanner(url, targetHost string, localPort int) {
	const (
		grn = "\x1b[1;32m"
		cyn = "\x1b[1;36m"
		dim = "\x1b[2m"
		rst = "\x1b[0m"
	)
	fmt.Println()
	fmt.Printf("  %s  LOCREST TUNNEL ACTIVE%s\n", grn, rst)
	fmt.Printf("  %s  URL:    %s%s%s\n", dim, cyn, url, rst)
	fmt.Printf("  %s  Source: %s%s:%d%s\n", dim, cyn, targetHost, localPort, rst)
	fmt.Printf("  %s  Press Ctrl+C to stop%s\n", dim, rst)
	fmt.Println()
}

// SuppressWriter filters log lines containing any of the configured substrings.
type SuppressWriter struct {
	W     io.Writer
	Hides []string
}

// NewSuppressWriter creates a writer that suppresses lines matching hides.
func NewSuppressWriter(w io.Writer, hides ...string) *SuppressWriter {
	return &SuppressWriter{W: w, Hides: hides}
}

func (s *SuppressWriter) Write(p []byte) (n int, err error) {
	line := string(p)
	for _, h := range s.Hides {
		if strings.Contains(line, h) {
			return len(p), nil
		}
	}
	return s.W.Write(p)
}
