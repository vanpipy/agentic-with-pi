package log

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

func CleanupOld(dir string, maxAgeDays int) {
	if maxAgeDays <= 0 {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -maxAgeDays)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".log") && !strings.HasSuffix(name, ".log.bak") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(dir, name))
		}
	}
}
