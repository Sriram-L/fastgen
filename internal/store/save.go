package store

import (
	"fmt"
	"os"
	"path/filepath"

	"fastgen/internal/runner"
)

// Save writes the failing input, expected output, and actual output to dir.
func Save(dir string, r runner.Result) ([]string, error) {
	if dir == "" {
		dir = "fastgen-fails"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	base := filepath.Join(dir, fmt.Sprintf("fail_%d", r.Seed))
	files := []struct {
		path string
		data []byte
	}{
		{base + ".in", r.Input},
		{base + ".expected", r.Expected},
		{base + ".got", r.Got},
	}

	var paths []string
	for _, f := range files {
		if err := os.WriteFile(f.path, f.data, 0o644); err != nil {
			return paths, err
		}
		paths = append(paths, f.path)
	}
	return paths, nil
}
