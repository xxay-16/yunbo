//go:build !windows

package transfer

import "os"

func openSource(path string) (*os.File, error) { return os.Open(path) }
