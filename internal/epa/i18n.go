package epa

import (
	"fmt"
	"sync/atomic"
)

var Languages = []string{"ko", "en"}

// EPA is a single-invocation CLI. Keep presentation language separate from
// operation identifiers and native process locales. Atomic reads also allow
// the terminal renderer to use the selected language safely.
var english atomic.Bool

func ValidLanguage(language string) bool {
	return language == "ko" || language == "en"
}

func SetLanguage(language string) bool {
	if !ValidLanguage(language) {
		return false
	}
	english.Store(language == "en")
	return true
}

func Language() string {
	if english.Load() {
		return "en"
	}
	return "ko"
}

func Text(ko, en string) string {
	if english.Load() {
		return en
	}
	return ko
}

// LocalizedErrorf retains Go error wrapping (including %w) while selecting
// a human-readable error message in the configured presentation language.
func LocalizedErrorf(ko, en string, args ...any) error {
	return fmt.Errorf(Text(ko, en), args...)
}
