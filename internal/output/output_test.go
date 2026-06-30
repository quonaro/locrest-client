package output

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"
)

func TestFormatTTL(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "unlimited"},
		{30 * time.Second, "30s"},
		{2 * time.Minute, "2m"},
		{90 * time.Second, "1m 30s"},
		{5*time.Minute + 1*time.Second, "5m 1s"},
	}
	for _, tt := range tests {
		if got := formatTTL(tt.d); got != tt.want {
			t.Fatalf("formatTTL(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestPrintBanner(t *testing.T) {
	old := osStdout
	defer func() { osStdout = old }()
	r, w := io.Pipe()
	osStdout = w
	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		done <- buf.String()
	}()

	PrintBanner("https://sub.example.com/", "localhost", 8080, time.Hour, "http", "user:pass", "alice")
	w.Close()
	out := <-done

	for _, want := range []string{
		"LOCREST TUNNEL ACTIVE",
		"https://sub.example.com/",
		"localhost:8080",
		"60m",
		"user:pass",
		"User:",
		"alice",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("banner missing %q: %s", want, out)
		}
	}
}

func TestPrintBannerTCP(t *testing.T) {
	old := osStdout
	defer func() { osStdout = old }()
	r, w := io.Pipe()
	osStdout = w
	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		done <- buf.String()
	}()

	PrintBanner("example.com:30001", "localhost", 8080, 0, "tcp", "", "")
	w.Close()
	out := <-done

	if !strings.Contains(out, "Dest:") {
		t.Fatalf("TCP banner should show Dest: %s", out)
	}
}

func TestSuppressWriter(t *testing.T) {
	var buf bytes.Buffer
	sw := NewSuppressWriter(&buf, "noise", "ignore")
	sw.Write([]byte("keep this\n"))
	sw.Write([]byte("noise line\n"))
	sw.Write([]byte("also ignore this\n"))
	sw.Write([]byte("keep that\n"))

	out := buf.String()
	if !strings.Contains(out, "keep this") {
		t.Fatalf("missing keep this: %q", out)
	}
	if !strings.Contains(out, "keep that") {
		t.Fatalf("missing keep that: %q", out)
	}
	if strings.Contains(out, "noise") {
		t.Fatalf("noise should be suppressed: %q", out)
	}
	if strings.Contains(out, "ignore") {
		t.Fatalf("ignore should be suppressed: %q", out)
	}
}

func TestSuppressWriterPartial(t *testing.T) {
	var buf bytes.Buffer
	sw := NewSuppressWriter(&buf, "drop")
	sw.Write([]byte("hello "))
	sw.Write([]byte("drop me\n"))
	sw.Write([]byte("world\n"))

	out := buf.String()
	if strings.Contains(out, "drop") {
		t.Fatalf("drop should be suppressed: %q", out)
	}
	if !strings.Contains(out, "world") {
		t.Fatalf("missing world: %q", out)
	}
}
