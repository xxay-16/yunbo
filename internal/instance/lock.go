// Package instance prevents concurrent writers to the same transfer checkpoint.
package instance

import (
	"os"
	"path/filepath"
)

func Acquire(profile string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(profile), 0700); err != nil {
		return nil, err
	}
	return acquire(profile)
}
