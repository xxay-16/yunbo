package clientapp

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/xxay-16/yunbo/internal/openlist"
)

type uploadItem struct {
	local, remote string
	isDir         bool
}

func planUploads(ctx context.Context, paths []string, remoteDir string, capacity int) ([]uploadItem, error) {
	var result []uploadItem
	seen := map[string]string{}
	files := 0
	appendItem := func(local, remote string, isDir bool) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if prior, exists := seen[remote]; exists {
			if prior == local {
				return nil
			}
			return fmt.Errorf("多个本机项目将上传到同一个位置：%s", remote)
		}
		if len(result) >= 4096 {
			return fmt.Errorf("此次拖入的目录项目过多，请分批上传")
		}
		if !isDir {
			files++
			if files > capacity {
				return fmt.Errorf("传输队列还可添加 %d 个文件，请清除已结束任务或分批上传", capacity)
			}
		}
		seen[remote] = local
		result = append(result, uploadItem{local: local, remote: remote, isDir: isDir})
		return nil
	}
	for _, input := range paths {
		if !filepath.IsAbs(input) {
			return nil, fmt.Errorf("文件路径无效，请重新选择文件")
		}
		local, err := filepath.Abs(input)
		if err != nil {
			return nil, err
		}
		stat, err := os.Lstat(local)
		if err != nil {
			return nil, err
		}
		base := filepath.Base(local)
		if err = openlist.ValidateName(base); err != nil {
			return nil, err
		}
		remote := path.Join(openlist.CleanPath(remoteDir), base)
		if !stat.IsDir() {
			if !stat.Mode().IsRegular() {
				return nil, fmt.Errorf("不支持上传符号链接或特殊文件：%s", local)
			}
			if err = appendItem(local, remote, false); err != nil {
				return nil, err
			}
			continue
		}
		err = filepath.WalkDir(local, func(p string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("目录包含符号链接，请单独选择要上传的普通文件：%s", p)
			}
			if !entry.IsDir() && !entry.Type().IsRegular() {
				return fmt.Errorf("不支持上传特殊文件：%s", p)
			}
			relative, err := filepath.Rel(local, p)
			if err != nil {
				return err
			}
			if relative != "." {
				for _, part := range strings.Split(filepath.ToSlash(relative), "/") {
					if err = openlist.ValidateName(part); err != nil {
						return err
					}
				}
			}
			target := remote
			if relative != "." {
				target = path.Join(remote, filepath.ToSlash(relative))
			}
			return appendItem(p, target, entry.IsDir())
		})
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}
