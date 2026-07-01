package supervisor

import (
	"bufio"
	"os"
	"strings"
)

const maxLogLines = 1000

func readLogLines(id string) []string {
	f, err := os.Open(DefaultLogPath())
	if err != nil {
		return []string{}
	}
	defer func() { _ = f.Close() }()

	var ring []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, id) {
			ring = append(ring, line)
			if len(ring) > maxLogLines {
				ring = ring[1:]
			}
		}
	}
	_ = scanner.Err()
	return ring
}
