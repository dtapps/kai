//go:build !darwin && !windows

package execkey

// copySelection 桩实现，仅保证跨平台编译通过。fallback 参数与真实平台签名保持一致。
func (e *ExecKeyController) copySelection(fallback bool) string {
	return ""
}
