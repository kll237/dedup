//go:build !windows

package suggest

import (
	"os"
	"time"
)

// accessTime returns the file's last-access time. On non-Windows platforms the
// standard library does not reliably expose atime, so we fall back to the
// modification time, which is always available.
func accessTime(fi os.FileInfo) time.Time {
	return fi.ModTime()
}
