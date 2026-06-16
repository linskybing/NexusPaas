package services

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServiceSourceDoesNotReadProcessEnv(t *testing.T) {
	root := "."
	var matches []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for lineNumber, line := range strings.Split(string(body), "\n") {
			if strings.Contains(line, "os.Getenv") || strings.Contains(line, "os.LookupEnv") {
				matches = append(matches, path+":"+itoa(lineNumber+1)+": "+strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) > 0 {
		t.Fatalf("non-test backend/internal/services files must use platform.Config instead of os.Getenv/os.LookupEnv:\n%s", strings.Join(matches, "\n"))
	}
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := [20]byte{}
	i := len(digits)
	for value > 0 {
		i--
		digits[i] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[i:])
}
