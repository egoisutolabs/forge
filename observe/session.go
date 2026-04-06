package observe

import (
	"os"
	"path/filepath"
	"sort"
	"time"
)

// RotateLogs removes session logs older than maxAge and enforces
// maxTotalBytes by deleting oldest files first.
// Called once at startup (best-effort, errors are silent).
func RotateLogs(logDir string, maxAge time.Duration, maxTotalBytes int64) {
	dir := logDir
	if dir == "" {
		var err error
		dir, err = logsDir()
		if err != nil {
			return
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	type logFile struct {
		path    string
		modTime time.Time
		size    int64
	}

	var files []logFile
	cutoff := time.Now().Add(-maxAge)

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		path := filepath.Join(dir, e.Name())

		// Delete files older than cutoff
		if info.ModTime().Before(cutoff) {
			os.Remove(path)
			continue
		}

		files = append(files, logFile{
			path:    path,
			modTime: info.ModTime(),
			size:    info.Size(),
		})
	}

	// Sort oldest first for size-based cleanup
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.Before(files[j].modTime)
	})

	var totalSize int64
	for _, f := range files {
		totalSize += f.size
	}

	// Delete oldest files until under size limit
	for totalSize > maxTotalBytes && len(files) > 0 {
		totalSize -= files[0].size
		os.Remove(files[0].path)
		files = files[1:]
	}
}
