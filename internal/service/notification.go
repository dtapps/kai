package service

import (
	"log/slog"

	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"cnb.cool/dtapp/kai/internal/i18n"
)

// NotificationService 封装原生桌面通知的授权检查与安全发送，
// 避免调用方直接裸调 Wails notifications service 时漏掉授权判断
// （macOS 上未授权会静默失败，且无法排查）。
type NotificationService struct {
	svc *notifications.NotificationService
}

// NewNotificationService 构造通知服务。svc 传 nil 时内部回退到
// notifications.NotificationService_ 单例，方便 main.go 直接复用已注册的服务实例。
func NewNotificationService(svc *notifications.NotificationService) *NotificationService {
	if svc == nil {
		svc = notifications.NotificationService_
	}
	return &NotificationService{svc: svc}
}

// ensureAuthorized 检查通知授权，未授权时主动申请。
// 返回是否最终获得授权；任何错误仅记录日志，不影响调用方主流程。
func (n *NotificationService) ensureAuthorized() bool {
	authorized, err := n.svc.CheckNotificationAuthorization()
	if err != nil {
		slog.Warn(i18n.T("log.notification_auth_request_failed"), "error", err)
	}
	if authorized {
		return true
	}
	if authorized, err = n.svc.RequestNotificationAuthorization(); err != nil {
		slog.Warn(i18n.T("log.notification_auth_request_failed"), "error", err)
	}
	return authorized
}

// Notify 安全发送通知：先确保授权，未授权或被拒则跳过并记录日志，
// 发送失败也只记日志，绝不向上抛出（更新检查等后台流程不应被通知问题阻断）。
func (n *NotificationService) Notify(opts notifications.NotificationOptions) {
	if !n.ensureAuthorized() {
		slog.Warn(i18n.T("log.notification_denied"))
		return
	}
	if err := n.svc.SendNotification(opts); err != nil {
		slog.Warn(i18n.T("log.send_update_notice_failed"), "error", err)
	}
}
