package ui

func (a *App) trayIcon(fallback []byte) []byte {
	if len(a.opts.TrayIcon) != 0 {
		return a.opts.TrayIcon
	}
	return fallback
}
