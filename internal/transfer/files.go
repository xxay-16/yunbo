package transfer

import (
	"errors"
	"fmt"
	"github.com/xxay-16/yunbo/internal/openlist"
	"os"
	"path/filepath"
	"strings"
)

func SafeLocalName(name string) error {
	if err := openlist.ValidateName(name); err != nil {
		return err
	}
	if strings.ContainsAny(name, "<>:\"|?*") || strings.TrimRight(name, ". ") != name {
		return errors.New("此文件名不能保存到 Windows，请先重命名")
	}
	base := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || (len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9') {
		return errors.New("文件名是 Windows 保留名称")
	}
	for _, r := range name {
		if r < 32 {
			return errors.New("文件名包含控制字符")
		}
	}
	return nil
}

func publish(temp, dir, name string) (string, error) {
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for i := 0; i < 10000; i++ {
		candidate := name
		if i > 0 {
			candidate = fmt.Sprintf("%s (%d)%s", stem, i, ext)
		}
		dst := filepath.Join(dir, candidate)
		// Reserve exclusively so parallel jobs and existing user files cannot be overwritten.
		reservation, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		if err = reservation.Close(); err != nil {
			os.Remove(dst)
			return "", err
		}
		if err = os.Rename(temp, dst); err != nil {
			os.Remove(dst)
			return "", err
		}
		return dst, nil
	}
	return "", errors.New("同名下载文件过多，请选择其他目录")
}
