//go:build !darwin && !windows

package execkey

// copySelection 桩实现，仅保证跨平台编译通过。
func (e *ExecKeyController) copySelection() string {
	return ""
}
