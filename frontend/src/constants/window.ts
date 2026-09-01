// 窗口名常量，与 internal/model/windows.go 对齐（单一真相源在 Go 端）。
// 用作 EventWindowClosing / EventWindowShow 等事件的窗口标识 payload。
export const WindowTranslate = 'translate';
export const WindowSettings = 'settings';
export const WindowScreenshot = 'screenshot';
