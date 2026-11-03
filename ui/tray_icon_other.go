//go:build !windows

package ui

func (a *App) trayIcon(fallback []byte) []byte { return fallback }
