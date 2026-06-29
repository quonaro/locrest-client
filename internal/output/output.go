package output

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// Fatal prints a formatted message to stderr and exits with code 1.
func Fatal(format string, v ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", v...)
	os.Exit(1)
}

func formatTTL(d time.Duration) string {
	if d == 0 {
		return "unlimited"
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	if m > 0 && s == 0 {
		return fmt.Sprintf("%dm", m)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// PrintBanner renders the tunnel activation banner.
func PrintBanner(url, targetHost string, localPort int, tokenTTL time.Duration) {
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
	fmt.Printf("  %s  TTL:    %s%s%s\n", dim, cyn, formatTTL(tokenTTL), rst)
	fmt.Printf("  %s  Press Ctrl+C to stop%s\n", dim, rst)
	fmt.Println()
}

// SuppressWriter filters log lines containing any of the configured substrings.
type SuppressWriter struct {
	W     io.Writer
	Hides []string
	buf   []byte
}

// NewSuppressWriter creates a writer that suppresses lines matching hides.
func NewSuppressWriter(w io.Writer, hides ...string) *SuppressWriter {
	return &SuppressWriter{W: w, Hides: hides}
}

func (s *SuppressWriter) Write(p []byte) (n int, err error) {
	s.buf = append(s.buf, p...)
	for {
		idx := bytes.IndexByte(s.buf, '\n')
		if idx < 0 {
			break
		}
		line := s.buf[:idx+1]
		drop := false
		for _, h := range s.Hides {
			if strings.Contains(string(line), h) {
				drop = true
				break
			}
		}
		if !drop {
			if _, err := s.W.Write(line); err != nil {
				return len(p), err
			}
		}
		s.buf = s.buf[idx+1:]
	}
	return len(p), nil
}
