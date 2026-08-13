//go:build windows

package selection

import (
	"syscall"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/wailsapp/wails/v3/pkg/application"
)

var (
	user32                  = syscall.NewLazyDLL("user32.dll")
	procGetForegroundWindow = user32.NewProc("GetForegroundWindow")
	procGetCursorPos        = user32.NewProc("GetCursorPos")
	procGetSystemMetrics    = user32.NewProc("GetSystemMetrics")
)

// UI Automation 常量
var (
	CLSID_CUIAutomation          = "{FF48DBA4-60EF-4201-AA87-54103EEF594E}"
	IID_IUIAutomation            = "{30CBE57D-D9D0-452A-AB13-7AC5AC4825EE}"
	IID_IUIAutomationTextPattern = "{32E26B4F-1C95-4D46-BE18-7E4D5C5D9B8C}"
)

const UIA_TextPatternId = 10014

// IUIAutomation COM 接口（vtbl 风格，仅实现本场景用到的方法）
type IUIAutomation struct {
	ole.IUnknown
	ElementFromHandle func(this uintptr, hwnd uintptr, element **IUIAutomationElement) uintptr
}

type IUIAutomationElement struct {
	ole.IUnknown
	GetCurrentPattern func(this uintptr, patternId uintptr, pattern **ole.IUnknown) uintptr
}

type IUIAutomationTextPattern struct {
	ole.IUnknown
	GetSelection func(this uintptr, ranges **IUIAutomationTextRange, count *int32) uintptr
}

type IUIAutomationTextRange struct {
	ole.IUnknown
	GetText func(this uintptr, maxLength int32, text **uint16) uintptr
}

// GetSelectedText 经 UI Automation 读取 Windows 前台窗口的选中文本（无需先复制）。
func GetSelectedText() string {
	ole.CoInitialize(0)
	defer ole.CoUninitialize()

	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return ""
	}

	clsid, err := ole.CLSIDFromString(CLSID_CUIAutomation)
	if err != nil {
		return ""
	}
	iid, err := ole.IIDFromString(IID_IUIAutomation)
	if err != nil {
		return ""
	}

	unknown, err := ole.CreateInstance(clsid, iid)
	if err != nil {
		return ""
	}
	defer unknown.Release()

	automation := (*IUIAutomation)(unsafe.Pointer(unknown))
	if automation.ElementFromHandle == nil {
		return ""
	}

	var element *IUIAutomationElement
	if automation.ElementFromHandle(uintptr(unsafe.Pointer(automation)), hwnd, &element); element == nil {
		return ""
	}
	defer element.Release()

	var pattern *ole.IUnknown
	if element.GetCurrentPattern(uintptr(unsafe.Pointer(element)), UIA_TextPatternId, &pattern); pattern == nil {
		return ""
	}
	defer pattern.Release()

	textPattern := (*IUIAutomationTextPattern)(unsafe.Pointer(pattern))
	if textPattern.GetSelection == nil {
		return ""
	}

	var ranges *IUIAutomationTextRange
	var count int32
	if textPattern.GetSelection(uintptr(unsafe.Pointer(textPattern)), &ranges, &count); ranges == nil || count == 0 {
		return ""
	}

	if ranges.GetText == nil {
		return ""
	}
	var textPtr *uint16
	if ranges.GetText(uintptr(unsafe.Pointer(ranges)), -1, &textPtr); textPtr == nil {
		return ""
	}
	text := ole.UTF16PtrToString(textPtr)
	return text
}

// currentSelectionOSA 供 currentSelection 在 windows 上优先经 UI Automation 读取选区。
func currentSelectionOSA() string {
	return GetSelectedText()
}

// currentSelectionPoint 用 user32 取鼠标坐标作为浮窗定位锚点。
func currentSelectionPoint() *application.Point {
	type point struct{ X, Y int32 }
	var p point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	return &application.Point{X: int(p.X), Y: int(p.Y)}
}

// primaryScreenSize 返回主屏幕分辨率。
func primaryScreenSize() (float64, float64) {
	const (
		SM_CXSCREEN = 0
		SM_CYSCREEN = 1
	)
	w, _, _ := procGetSystemMetrics.Call(SM_CXSCREEN)
	h, _, _ := procGetSystemMetrics.Call(SM_CYSCREEN)
	return float64(w), float64(h)
}
