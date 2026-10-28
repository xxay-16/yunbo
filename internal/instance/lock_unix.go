//go:build !windows

package instance

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"syscall"
)

func acquire(profile string) (func(), error) {
	file, err := os.OpenFile(filepath.Join(filepath.Dir(profile), ".instance.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		return nil, errors.New("Fyne 版云泊已在运行，请从系统托盘恢复窗口")
	}
	var once sync.Once
	return func() { once.Do(func() { syscall.Flock(int(file.Fd()), syscall.LOCK_UN); file.Close() }) }, nil
}
