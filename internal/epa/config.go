package epa

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type Config struct {
	Style      string `json:"style"`
	Progress   string `json:"progress,omitempty"`
	SelfUpdate bool   `json:"self_update"`
	Language   string `json:"language"`
}

func accountConfigDir() (string, error) {
	dir, err := os.UserConfigDir()
	if inheritedRootEnvironment() {
		home, homeErr := accountHome()
		if homeErr != nil {
			return "", homeErr
		}
		if err := safeCompletionDirectory(home); err != nil {
			return "", err
		}
		dir, err = filepath.Join(home, ".config"), nil
		// Do not follow an account's symlink or shared writable config dir.
		if err := safeCompletionDirectory(dir); err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(dir) {
		return "", LocalizedErrorf("설정 디렉터리는 절대 경로여야 합니다.", "configuration directory must be absolute")
	}
	return filepath.Clean(dir), nil
}

func configPath() (string, error) {
	dir, err := accountConfigDir()
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, "epa")
	path := filepath.Join(dir, "config.json")
	if inheritedRootEnvironment() {
		if err := safeCompletionDirectory(dir); err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		if info, err := os.Lstat(path); err == nil {
			if !safeCompletionFile(info) {
				return "", LocalizedErrorf("안전하지 않은 설정 파일입니다: %s", "unsafe configuration file: %s", path)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}
	return path, nil
}

func LoadConfig() (Config, error) {
	c := Config{Style: "apt", Progress: "auto", SelfUpdate: true, Language: "ko"}
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
		return c, LocalizedErrorf("EPA 설정이 올바르지 않습니다: %w", "invalid EPA config: %w", err)
	}
	if !ValidStyle(c.Style) {
		return c, LocalizedErrorf("설정된 명령 스타일이 올바르지 않습니다: %q", "invalid configured style %q", c.Style)
	}
	if !ValidProgressMode(c.Progress) {
		return c, LocalizedErrorf("설정된 진행 표시 방식이 올바르지 않습니다: %q", "invalid configured progress mode %q", c.Progress)
	}
	if !ValidLanguage(c.Language) {
		return c, LocalizedErrorf("설정된 언어가 올바르지 않습니다: %q", "invalid configured language %q", c.Language)
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
		return LocalizedErrorf("알 수 없는 명령 스타일입니다: %q", "unknown style %q", style)
	}
	c, err := LoadConfig()
	if err != nil {
		return err
	}
	c.Style = style
	return saveConfig(c)
}

func ResetStyle() error { return SaveStyle("apt") }

func SaveSelfUpdate(enabled bool) error {
	c, err := LoadConfig()
	if err != nil {
		return err
	}
	c.SelfUpdate = enabled
	return saveConfig(c)
}

func SaveLanguage(language string) error {
	if !ValidLanguage(language) {
		return LocalizedErrorf("지원하지 않는 언어입니다: %q (ko, en)", "unsupported language %q (ko, en)", language)
	}
	c, err := LoadConfig()
	if err != nil {
		return err
	}
	c.Language = language
	return saveConfig(c)
}

func SaveProgress(mode string) error {
	if !ValidProgressMode(mode) {
		return LocalizedErrorf("알 수 없는 진행 표시 방식입니다: %q", "unknown progress mode %q", mode)
	}
	c, err := LoadConfig()
	if err != nil {
		return err
	}
	c.Progress = mode
	return saveConfig(c)
}
