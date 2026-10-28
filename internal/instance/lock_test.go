package instance

import (
	"path/filepath"
	"testing"
)

func TestSingleCheckpointWriterAndRelease(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "profile.json")
	release, err := Acquire(profile)
	if err != nil {
		t.Fatal(err)
	}
	if second, err := Acquire(profile); err == nil {
		second()
		release()
		t.Fatal("second writer acquired the same profile")
	}
	release()
	again, err := Acquire(profile)
	if err != nil {
		t.Fatal(err)
	}
	again()
}
