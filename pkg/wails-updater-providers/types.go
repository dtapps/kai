package wails_updater_providers

// ===================== CNB 源 =====================

// cnbReleaseListItem CNB releases 列表项（对应 CNB API 的 api.Release 元素）。
type cnbReleaseListItem struct {
	TagName     string            `json:"tag_name"`     // 版本标签（可能带 v 前缀）
	Name        string            `json:"name"`         // 发布标题
	Body        string            `json:"body"`         // 发布说明（描述）
	Prerelease  bool              `json:"prerelease"`   // 是否为预发布
	Draft       bool              `json:"draft"`        // 是否为草稿
	PublishedAt string            `json:"published_at"` // 发布时间（RFC3339）
	CreatedAt   string            `json:"created_at"`   // 创建时间（RFC3339）
	IsLatest    bool              `json:"is_latest"`    // 是否为最新
	Assets      []cnbReleaseAsset `json:"assets"`       // 资源列表
}

// GetPublishedAt 实现 publishedAtGetter 接口，供泛型排序函数使用。
func (r cnbReleaseListItem) GetPublishedAt() string { return r.PublishedAt }

// cnbReleaseTagDetail CNB 单个 release 详情（GetReleaseByTag 响应）。
// 与 GitHub 详情的差异：CNB 额外返回 ID / TagCommitish 字段，GitHub 无。
type cnbReleaseTagDetail struct {
	ID           string            `json:"id"`            // CNB 专属：release 唯一 ID（GitHub 无，API 以字符串返回）
	TagName      string            `json:"tag_name"`      // 版本标签
	TagCommitish string            `json:"tag_commitish"` // CNB 专属：关联提交（GitHub 用 target_commitish）
	Name         string            `json:"name"`          // 发布标题
	Body         string            `json:"body"`          // 发布说明
	Prerelease   bool              `json:"prerelease"`    // 是否为预发布
	PublishedAt  string            `json:"published_at"`  // 发布时间（RFC3339）
	Assets       []cnbReleaseAsset `json:"assets"`        // 资源列表
}

// cnbReleaseAsset CNB release 的单个资源（升级产物候选，CNB 专属内部类型）。
// 注意：CNB 与 GitHub 的资源字段（name/size）表面一致，但分属两个独立 API，
// 保留各自类型以避免日后两源响应结构分化时互相污染。
type cnbReleaseAsset struct {
	Name string `json:"name"` // 文件名
	Size int64  `json:"size"` // 文件大小
}

// ===================== GitHub 源 =====================

// githubRelease GitHub release 响应（对应 GitHub API 的 release 对象）。
type githubRelease struct {
	TagName         string        `json:"tag_name"`         // 版本标签
	TargetCommitish string        `json:"target_commitish"` // 目标提交（用于 nightly 同 commit 跳过）
	Name            string        `json:"name"`             // 发布标题
	Body            string        `json:"body"`             // 发布说明
	Draft           bool          `json:"draft"`            // 是否为草稿
	Prerelease      bool          `json:"prerelease"`       // 是否为预发布
	HTMLURL         string        `json:"html_url"`         // 发布页地址
	PublishedAt     string        `json:"published_at"`     // 发布时间（RFC3339）
	Assets          []githubAsset `json:"assets"`           // 资源列表
}

// GetPublishedAt 实现 publishedAtGetter 接口，供泛型排序函数使用。
func (r githubRelease) GetPublishedAt() string { return r.PublishedAt }

// githubAsset GitHub release 的单个资源。
type githubAsset struct {
	Name string `json:"name"` // 文件名
	Size int64  `json:"size"` // 文件大小
}
