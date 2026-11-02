//go:build !windows

package main

import (
	"errors"
	"github.com/zigorminsk-debug/esxi/raid-assistant/internal/app"
)

func runWindow(_ *app.App) error {
	return errors.New("The desktop application is Windows-only; use cmd/preview for a mock browser preview")
}
