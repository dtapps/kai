import { Events, Window } from '@wailsio/runtime';
import { EventNotification, type NotificationPayload } from '../utils/events';

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

// 后端更新通知（kai:notification）转系统通知（对齐 certflow：检查到新版本弹桌面提示）。
// 模块被各窗口共享，仅注册一次。
if (typeof window !== 'undefined' && 'Notification' in window) {
  Events.On(EventNotification, (e: any) => {
    const p = e?.data as NotificationPayload | undefined;
    if (!p) return;
    const show = () => {
      try {
        new Notification(p.title, { body: [p.subtitle, p.body].filter(Boolean).join('\n') });
      } catch {
        /* 忽略通知权限/构造失败 */
      }
    };
    if (window.Notification.permission === 'granted') {
      show();
    } else if (window.Notification.permission !== 'denied') {
      window.Notification.requestPermission().then((perm) => {
        if (perm === 'granted') show();
      });
    }
  });
}
