package desktop

// SetRunningForTest exposes isRunning for unit tests in desktop_test package.
func (a *DesktopApp) SetRunningForTest(running bool) {
	a.cancelMu.Lock()
	defer a.cancelMu.Unlock()
	a.isRunning = running
}
