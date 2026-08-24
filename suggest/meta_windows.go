//go:build windows

package suggest

import (
	"os"
	"syscall"
	"time"
)

// accessTime returns the file's last-access (last used) time. On Windows the
// FileInfo returned by os.Stat carries a *syscall.Win32FileAttributeData in its
// Sys() field, which exposes LastAccessTime with nanosecond precision.
func accessTime(fi os.FileInfo) time.Time {
	if w, ok := fi.Sys().(*syscall.Win32FileAttributeData); ok {
		return time.Unix(0, w.LastAccessTime.Nanoseconds())
	}
	return fi.ModTime()
}
