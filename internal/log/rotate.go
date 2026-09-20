package log

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func Rotate(path string, maxBytes int64) error {
	info, err := os.Stat(path)
	if err != nil || info.Size() < maxBytes {
		return nil
	}

	ts := time.Now().UTC().Format("20060102T150405")
	base := strings.TrimSuffix(path, filepath.Ext(path))
	backup := fmt.Sprintf("%s-%s.log.bak", base, ts)

	return os.Rename(path, backup)
}
