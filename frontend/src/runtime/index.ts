import { Events, Window } from '@wailsio/runtime';

export async function getWindowName(): Promise<string> {
  return Window.Name();
}

export function onEvent(name: string, cb: (data: any) => void): () => void {
  return Events.On(name, (e: any) => cb(e.data));
}

export function emitEvent(name: string, data?: any): void {
  Events.Emit(name, data);
}

export { Window };

// 桌面通知由后端 Wails notifications service 直接发送原生通知（macOS 走 UNUserNotificationCenter），
// 不再经前端 Web Notification 转发（前端在后台时不会弹窗，且绕开了系统通知授权）。
