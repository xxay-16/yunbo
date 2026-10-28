//go:build !windows && !darwin

package tray

func start(_ []byte, _ Callbacks) func() { return func() {} }
