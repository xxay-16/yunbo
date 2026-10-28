package nativeui

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/xxay-16/yunbo/internal/openlist"
	"github.com/xxay-16/yunbo/internal/transfer"
)

func formatBytes(n int64) string {
	if n < 0 {
		return "—"
	}
	if n == 0 {
		return "0 B"
	}
	unit := min(int(math.Log(float64(n))/math.Log(1024)), 4)
	return fmt.Sprintf("%.1f %s", float64(n)/math.Pow(1024, float64(unit)), []string{"B", "KB", "MB", "GB", "TB"}[unit])
}

func formatETA(seconds int64) string {
	if seconds < 0 {
		return "剩余时间未知"
	}
	if seconds < 60 {
		return fmt.Sprintf("剩余 %d 秒", seconds)
	}
	minutes := (seconds + 59) / 60
	if minutes < 60 {
		return fmt.Sprintf("剩余 %d 分钟", minutes)
	}
	return fmt.Sprintf("剩余 %d 小时 %d 分钟", minutes/60, minutes%60)
}

func formatDate(value string) string {
	if t, err := time.Parse(time.RFC3339Nano, value); err == nil && t.Year() > 1970 {
		return t.Local().Format("2006-01-02 15:04")
	}
	return "—"
}

func visibleFiles(files []openlist.File, filter, order string, ascending bool) []openlist.File {
	result := make([]openlist.File, 0, len(files))
	for _, file := range files {
		if strings.Contains(strings.ToLower(file.Name), strings.ToLower(filter)) {
			result = append(result, file)
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if a.IsDir != b.IsDir {
			return a.IsDir
		}
		cmp := strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
		if order == "size" {
			if a.Size < b.Size {
				cmp = -1
			} else if a.Size > b.Size {
				cmp = 1
			} else {
				cmp = 0
			}
		}
		if order == "modified" {
			cmp = strings.Compare(a.Modified, b.Modified)
		}
		if ascending {
			return cmp < 0
		}
		return cmp > 0
	})
	return result
}

func taskStatus(task transfer.Task) string {
	if task.Status == "running" {
		if task.Phase == "checking" {
			if task.Direction == "upload" && task.Resumable {
				return fmt.Sprintf("校验源文件 %d%%", task.PhaseProgress)
			}
			return "准备中"
		}
		if task.Phase == "committing" {
			return "等待服务端确认"
		}
	}
	return map[string]string{"queued": "排队中", "running": "传输中", "pausing": "正在暂停", "paused": "已暂停", "cancelling": "正在取消", "cancelled": "已取消", "completed": "已完成", "failed": "失败"}[task.Status]
}
