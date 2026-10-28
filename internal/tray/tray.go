// Package tray integrates with each desktop's existing application event loop.
package tray

type Callbacks struct {
	Ready      func()
	Show       func()
	Background func()
	OpenConfig func()
	Quit       func()
}

// Start creates one tray icon per process. The caller may hide the window only
// after Ready is called. The returned function removes the icon on shutdown.
func Start(icon []byte, callbacks Callbacks) func() { return start(icon, callbacks) }
