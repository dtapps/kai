//go:build windows

package selection

// isAccessibilityEnabled Windows 下 UI Automation 无需额外授权开关，恒为真。
func isAccessibilityEnabled() bool {
	return true
}
