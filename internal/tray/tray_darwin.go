package tray

/*
#cgo LDFLAGS: -framework Cocoa
void yunboTrayStart(void);
void yunboTrayStop(void);
*/
import "C"

import "sync"

var callbackState struct {
	sync.RWMutex
	callbacks Callbacks
	stopped   bool
}

func start(_ []byte, callbacks Callbacks) func() {
	callbackState.Lock()
	callbackState.callbacks = callbacks
	callbackState.stopped = false
	callbackState.Unlock()
	C.yunboTrayStart()
	var once sync.Once
	return func() {
		once.Do(func() {
			callbackState.Lock()
			callbackState.stopped = true
			callbackState.Unlock()
			C.yunboTrayStop()
		})
	}
}

//export yunboTrayDispatch
func yunboTrayDispatch(event C.int) {
	callbackState.RLock()
	callbacks, stopped := callbackState.callbacks, callbackState.stopped
	callbackState.RUnlock()
	if stopped {
		return
	}
	var action func()
	switch int(event) {
	case 0:
		action = callbacks.Ready
	case 1:
		action = callbacks.Show
	case 2:
		action = callbacks.Background
	case 3:
		action = callbacks.OpenConfig
	case 4:
		action = callbacks.Quit
	}
	if action != nil {
		go action()
	}
}
