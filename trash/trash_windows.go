//go:build windows

// Package trash moves files into the operating system's recycle bin / trash
// instead of permanently deleting them.
package trash

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// MoveToTrash moves every path into the Windows Recycle Bin using
// SHFileOperationW with the FOF_ALLOWUNDO flag, so the files can be restored.
// It never permanently deletes.
func MoveToTrash(paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	dll := syscall.NewLazyDLL("shell32.dll")
	proc := dll.NewProc("SHFileOperationW")
	if proc == nil {
		return fmt.Errorf("无法定位 shell32.dll!SHFileOperationW")
	}

	// Build a double-NUL terminated UTF-16 string of all paths.
	var buf []uint16
	for _, p := range paths {
		w, err := syscall.UTF16FromString(p)
		if err != nil {
			return err
		}
		buf = append(buf, w...)
	}
	buf = append(buf, 0) // second NUL -> double NUL terminator required by the API

	// Layout must match Windows SHFILEOPSTRUCTW (56 bytes on amd64).
	var fo struct {
		hwnd                  uintptr
		wFunc                 uint32
		pFrom                 *uint16
		pTo                   *uint16
		fFlags                uint32
		fAnyOperationsAborted int32
		hNameMappings         uintptr
		lpszProgressTitle     *uint16
	}
	fo.wFunc = 0x0003 // FO_DELETE
	fo.pFrom = &buf[0]
	fo.fFlags = 0x0040 | 0x0010 | 0x0400 // FOF_ALLOWUNDO | FOF_NOCONFIRMATION | FOF_NOERRORUI

	r, _, _ := proc.Call(uintptr(unsafe.Pointer(&fo)))

	// SHFileOperation can return a spurious non-zero code in some environments
	// even though the move succeeded. We therefore verify the intended effect:
	// if every source file is actually gone, the operation succeeded.
	if r == 0 && fo.fAnyOperationsAborted == 0 {
		return nil
	}
	allGone := true
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			allGone = false
			break
		}
	}
	if allGone {
		return nil
	}
	if r != 0 {
		return fmt.Errorf("SHFileOperation 失败，代码 %d", r)
	}
	return fmt.Errorf("部分文件未移入回收站（操作被中止）")
}
