package service

import (
	"context"
	"time"

	"cnb.cool/dtapp/kai/internal/configstore"
	"cnb.cool/dtapp/kai/internal/historystore"
)

// HistoryItem 返回给前端的单条历史（时间转为毫秒时间戳，引擎 ID 转为可读名）。
type HistoryItem struct {
	ID        int64  `json:"id"`         // 历史记录自增主键 ID
	Text      string `json:"text"`       // 原文
	Result    string `json:"result"`     // 翻译结果
	FromLang  string `json:"from_lang"`  // 源语言代码
	ToLang    string `json:"to_lang"`    // 目标语言代码
	Engine    string `json:"engine"`     // 使用的引擎标识
	FromOCR   bool   `json:"from_ocr"`   // 是否来自 OCR 识别结果
	CreatedAt int64  `json:"created_at"` // 创建时间（毫秒时间戳）
}

// HistoryWrapper 翻译历史的薄适配层：持有 historystore.Store + configstore，仅做 RPC 透传与 DTO 转换。
// 不实现 wails 生命周期三件套。
type HistoryWrapper struct {
	store       *historystore.Store
	configStore *configstore.Store
}

// NewHistoryWrapper 构造历史 Wrapper。
func NewHistoryWrapper(store *historystore.Store, cs *configstore.Store) *HistoryWrapper {
	return &HistoryWrapper{store: store, configStore: cs}
}

// GetHistory 分页查询翻译历史。
func (w *HistoryWrapper) GetHistory(keyword string, offset, limit int) []HistoryItem {
	if w.store == nil {
		return nil
	}
	if limit <= 0 {
		limit = 50
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, err := w.store.QueryByKeyword(ctx, keyword, int64(limit), int64(offset))
	if err != nil {
		return nil
	}
	out := make([]HistoryItem, 0, len(rows))
	for _, r := range rows {
		engineName := ""
		if w.configStore != nil && r.EngineID != 0 {
			if eng, e := w.configStore.GetEngineByID(ctx, r.EngineID); e == nil && eng != nil {
				engineName = eng.Engine
			}
		}
		out = append(out, HistoryItem{
			ID:        r.ID,
			Text:      r.Text,
			Result:    r.Result,
			FromLang:  r.FromLang,
			ToLang:    r.ToLang,
			Engine:    engineName,
			FromOCR:   r.FromOcr != 0,
			CreatedAt: r.CreatedAt,
		})
	}
	return out
}

// CountHistory 返回符合关键词的历史总条数，供前端分页计算总页数。
func (w *HistoryWrapper) CountHistory(keyword string) int64 {
	if w.store == nil {
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	n, err := w.store.CountByKeyword(ctx, keyword)
	if err != nil {
		return 0
	}
	return n
}

// DeleteHistory 删除一条历史。
func (w *HistoryWrapper) DeleteHistory(id int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return w.store.DeleteHistory(ctx, id)
}

// ClearHistory 清空历史。
func (w *HistoryWrapper) ClearHistory() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return w.store.ClearHistory(ctx)
}
