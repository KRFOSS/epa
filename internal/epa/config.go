package epa

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	Style    string `json:"style"`
	Progress string `json:"progress,omitempty"`
}

func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(dir) {
		return "", fmt.Errorf("configuration directory must be absolute")
	}
	return filepath.Join(dir, "epa", "config.json"), nil
}

func LoadConfig() (Config, error) {
	c := Config{Style: "apt", Progress: "auto"}
	path, err := configPath()
	if err != nil {
		return c, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return c, fmt.Errorf("invalid EPA config: %w", err)
	}
	if !ValidStyle(c.Style) {
		return c, fmt.Errorf("invalid configured style %q", c.Style)
	}
	if !ValidProgressMode(c.Progress) {
		return c, fmt.Errorf("invalid configured progress mode %q", c.Progress)
	}
	return c, nil
}

func LoadStyle() (string, error) {
	c, err := LoadConfig()
	return c.Style, err
}

func saveConfig(c Config) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".config-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(append(data, '\n')); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func SaveStyle(style string) error {
	if !ValidStyle(style) {
		return fmt.Errorf("unknown style %q", style)
	}
	c, err := LoadConfig()
	if err != nil {
		return err
	}
	c.Style = style
	return saveConfig(c)
}

func ResetStyle() error { return SaveStyle("apt") }

func SaveProgress(mode string) error {
	if !ValidProgressMode(mode) {
		return fmt.Errorf("unknown progress mode %q", mode)
	}
	c, err := LoadConfig()
	if err != nil {
		return err
	}
	c.Progress = mode
	return saveConfig(c)
}
