//go:build !windows

package trash

import (
	"fmt"
	"os/exec"
)

// MoveToTrash moves files into the system trash on non-Windows platforms.
// It prefers `gio trash` / `trash-put` and, failing that, refuses to delete
// anything so user data is never lost by this tool.
func MoveToTrash(paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	if _, err := exec.LookPath("gio"); err == nil {
		if c := exec.Command("gio", append([]string{"trash"}, paths...)...); c.Run() == nil {
			return nil
		}
	}
	if _, err := exec.LookPath("trash-put"); err == nil {
		if c := exec.Command("trash-put", paths...); c.Run() == nil {
			return nil
		}
	}
	return fmt.Errorf("当前系统未找到回收站命令（gio/trash-put），为安全起见未删除任何文件")
}
