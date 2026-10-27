package transfer

import (
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"strings"
)

// Keep the file stable while hashing and streaming chunks. Another reader is
// allowed, but changing or deleting the source cannot silently corrupt a session.
func openSource(path string) (*os.File, error) {
	path = filepath.Clean(path)
	if !strings.HasPrefix(path, `\\?\`) {
		if strings.HasPrefix(path, `\\`) {
			path = `\\?\UNC\` + strings.TrimPrefix(path, `\\`)
		} else {
			path = `\\?\` + path
		}
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_SEQUENTIAL_SCAN, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(handle), path), nil
}
