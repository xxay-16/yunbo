package instance

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"

	"golang.org/x/sys/windows"
)

func acquire(profile string) (func(), error) {
	key := sha256.Sum256([]byte(profile))
	name, _ := windows.UTF16PtrFromString(fmt.Sprintf("Local\\YunboFyne-%x", key[:16]))
	handle, err := windows.CreateMutex(nil, false, name)
	if err != nil {
		if handle != 0 {
			windows.CloseHandle(handle)
		}
		if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
			return nil, errors.New("Fyne 版云泊已在运行，请从系统托盘恢复窗口")
		}
		return nil, err
	}
	var once sync.Once
	return func() { once.Do(func() { windows.CloseHandle(handle) }) }, nil
}
