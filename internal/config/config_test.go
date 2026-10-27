package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestProfileRoundtripAndReplacement(t *testing.T) {
	file := filepath.Join(t.TempDir(), "config", "profile.json")
	profile, err := Load(file)
	if err != nil || profile.StartPath != "/" {
		t.Fatalf("unexpected default: %#v %v", profile, err)
	}
	want := Profile{URL: "https://cloud.example.com/openlist", Username: "alice", StartPath: "/中文", AuthMode: "password"}
	for _, user := range []string{"alice", "bob"} {
		want.Username = user
		if err = Save(file, want); err != nil {
			t.Fatal(err)
		}
	}
	got, err := Load(file)
	if err != nil || !reflect.DeepEqual(want, got) {
		t.Fatalf("roundtrip failed: %#v %v", got, err)
	}
	data, _ := os.ReadFile(file)
	var fields map[string]any
	if err = json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"password", "token", "otp", "directory_password"} {
		if _, exists := fields[secret]; exists {
			t.Fatalf("profile contains credential field %s", secret)
		}
	}
	if err = os.WriteFile(file, []byte("invalid JSON"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = Load(file); err == nil {
		t.Fatal("corrupt configuration was silently ignored")
	}
}
