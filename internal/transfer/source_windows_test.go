package transfer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUploadSourceCannotBeModifiedWhileOpen(t *testing.T) {
	file := filepath.Join(t.TempDir(), "source.bin")
	if err := os.WriteFile(file, []byte("stable"), 0600); err != nil {
		t.Fatal(err)
	}
	source, err := openSource(file)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	if writer, err := os.OpenFile(file, os.O_WRONLY, 0600); err == nil {
		writer.Close()
		t.Fatal("active upload source allowed modification")
	}
}
