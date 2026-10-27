package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Profile deliberately excludes passwords, OTP codes, tokens and directory passwords.
type Profile struct {
	URL       string `json:"url"`
	Username  string `json:"username"`
	StartPath string `json:"start_path"`
	AuthMode  string `json:"auth_mode"`
}

func Location() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "openlist-desk", "profile.json"), nil
}

func Load(file string) (Profile, error) {
	var p Profile
	data, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return Profile{StartPath: "/", AuthMode: "password"}, nil
	}
	if err != nil {
		return p, err
	}
	err = json.Unmarshal(data, &p)
	return p, err
}

func Save(file string, p Profile) error {
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(file), ".profile-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(b)
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), file)
}
