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

// FatalCode prints a formatted message to stderr and exits with the given code.
func FatalCode(code int, format string, v ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", v...)
	os.Exit(code)
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
		red = "\x1b[1;31m"
		dim = "\x1b[2m"
		rst = "\x1b[0m"
	)
	fmt.Println()
	fmt.Printf("%sLOCREST TUNNEL ACTIVE%s\n", grn, rst)
	fmt.Printf("%sURL:    %s%s%s\n", dim, cyn, url, rst)
	fmt.Printf("%sSource: %s%s:%d%s\n", dim, cyn, targetHost, localPort, rst)
	fmt.Printf("%sTTL:    %s%s%s\n", dim, cyn, formatTTL(tokenTTL), rst)
	fmt.Printf("%sPress Ctrl+C to stop%s\n", red, rst)
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

func (s *SuppressWriter) Write(p []byte) (int, error) {
	s.buf = append(s.buf, p...)
	written := 0
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
				return written, err
			}
		}
		written += len(line)
		s.buf = s.buf[idx+1:]
	}
	return len(p), nil
}
