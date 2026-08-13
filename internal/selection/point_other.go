//go:build !darwin && !windows

package selection

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// currentSelectionPoint 非 darwin/windows 平台暂不支持选区坐标，返回 nil。
func currentSelectionPoint() *application.Point {
	return nil
}

// primaryScreenSize 非 darwin/windows 平台返回 0,0。
func primaryScreenSize() (float64, float64) {
	return 0, 0
}

// currentSelectionOSA 非 darwin/windows 平台返回空串（无系统取词）。
func currentSelectionOSA() string {
	return ""
}
