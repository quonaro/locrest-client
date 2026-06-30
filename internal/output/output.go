package output

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// osStdout allows tests to capture banner output.
var osStdout io.Writer = os.Stdout

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

// FatalRed prints a formatted message in red to stderr and exits with the given code.
func FatalRed(code int, format string, v ...interface{}) {
	fmt.Fprintf(os.Stderr, "%s", ansiBoldRed)
	fmt.Fprintf(os.Stderr, format+"\n", v...)
	fmt.Fprintf(os.Stderr, "%s", ansiReset)
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

const (
	ansiBoldGreen = "\x1b[1;32m"
	ansiBoldCyan  = "\x1b[1;36m"
	ansiBoldRed   = "\x1b[1;31m"
	ansiDim       = "\x1b[2m"
	ansiReset     = "\x1b[0m"
)

// PrintBanner renders the tunnel activation banner.
func PrintBanner(url, targetHost string, localPort int, tokenTTL time.Duration, mode, httpAuth string, authorized bool) {
	fmt.Fprintln(osStdout)
	fmt.Fprintf(osStdout, "%sLOCREST TUNNEL ACTIVE%s\n", ansiBoldGreen, ansiReset)
	if mode == "tcp" {
		fmt.Fprintf(osStdout, "%sDest:   %s%s%s\n", ansiDim, ansiBoldCyan, url, ansiReset)
	} else {
		fmt.Fprintf(osStdout, "%sURL:    %s%s%s\n", ansiDim, ansiBoldCyan, url, ansiReset)
	}
	if httpAuth != "" {
		fmt.Fprintf(osStdout, "%sAuth:   %s%s%s\n", ansiDim, ansiBoldCyan, httpAuth, ansiReset)
	}
	fmt.Fprintf(osStdout, "%sSource: %s%s:%d%s\n", ansiDim, ansiBoldCyan, targetHost, localPort, ansiReset)
	fmt.Fprintf(osStdout, "%sTTL:    %s%s%s\n", ansiDim, ansiBoldCyan, formatTTL(tokenTTL), ansiReset)
	if authorized {
		fmt.Fprintf(osStdout, "%sSession: %sauthorized%s\n", ansiDim, ansiBoldCyan, ansiReset)
	}
	fmt.Fprintf(osStdout, "%sPress Ctrl+C to stop%s\n", ansiBoldRed, ansiReset)
	fmt.Fprintln(osStdout)
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
