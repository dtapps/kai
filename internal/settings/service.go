package settings

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"

	"cnb.cool/dtapp/kai/internal/model"
)

// Settings 应用设置（仅界面偏好，引擎配置在 config.db 独立存储，不在此）。
type Settings struct {
	// Language 界面语言：auto / zh-CN / en-US（auto 由系统语言解析）。
	// 注意：这是应用界面显示语言，与翻译语言（model.Language：auto/zh/en/...）是两套独立体系，切勿混用。
	Language string `json:"language" mapstructure:"language"`
	// Theme 界面主题：auto / light / dark（auto 跟随系统外观）。
	Theme string `json:"theme" mapstructure:"theme"`
	// DefaultTo 翻译默认目标语言（如 zh / en）。
	DefaultTo string `json:"default_to" mapstructure:"default_to"`
	// DefaultFrom 翻译默认源语言（auto 表示自动检测）。
	DefaultFrom string `json:"default_from" mapstructure:"default_from"`
	// Hotkeys 注册类全局快捷键（用户按下即触发动作）。
	Hotkeys RegisteredHotkeyConfig `json:"hotkeys" mapstructure:"hotkeys"`
	// ExecKeys 执行类快捷键（程序主动模拟按下，用于完成动作）。
	ExecKeys ExecKeyConfig `json:"execkeys" mapstructure:"execkeys"`
	// TTS 语音合成配置。
	TTS TTSConfig `json:"tts" mapstructure:"tts"`
	// HttpLog HTTP 请求日志配置（暂不同步到前端 UI）。
	HttpLog HttpLogConfig `json:"http_log" mapstructure:"http_log"`
	// Log 应用运行日志配置（等级 / 清理 / 压缩）。
	Log LogConfig `json:"log" mapstructure:"log"`
	// Proxy 网络代理配置（暂不同步到前端 UI）。
	Proxy ProxyConfig `json:"proxy" mapstructure:"proxy"`
	// Updater 自动更新配置（仅用户可决策项，token/provider 由代码固定）。
	Updater UpdaterConfig `json:"updater" mapstructure:"updater"`
	// DNSConfigs 自定义 DNS 解析配置列表。
	DNSConfigs []DNSConfig `json:"dns_configs" mapstructure:"dns_configs"`
	// Path 配置文件路径（不持久化到文件，json:"-"）。
	Path string `json:"-"`
}

// HotkeyEntry 单个注册类快捷键：既保存按键组合，也保存启用状态。
// 之前结构只存按键字符串、缺少逐项启用状态；现拆为 Key（按键，空串表示未设置）
// 与 Enabled（是否启用）。未启用或 Key 为空都不注册。
type HotkeyEntry struct {
	// Key 按键组合，空串表示未设置。
	Key string `json:"key" mapstructure:"key"`
	// Enabled 是否启用该快捷键。
	Enabled bool `json:"enabled" mapstructure:"enabled"`
}

// RegisteredHotkeyConfig 注册类全局快捷键（均通过 mgr.Register 监听，用户按下即触发动作）。
// 每个快捷键是 HotkeyEntry，含按键与启用状态。
type RegisteredHotkeyConfig struct {
	// Input 唤起主窗口快捷键（核心功能，默认启用）。
	Input HotkeyEntry `json:"input" mapstructure:"input"`
	// Screenshot 截图翻译快捷键。
	Screenshot HotkeyEntry `json:"screenshot" mapstructure:"screenshot"`
}

// ExecKeyEntry 单个执行类快捷键：程序主动模拟按下的键 + 是否启用。
type ExecKeyEntry struct {
	// Key 程序主动模拟按下的按键组合。
	Key string `json:"key" mapstructure:"key"`
	// Enabled 是否启用该执行键。
	Enabled bool `json:"enabled" mapstructure:"enabled"`
	// Fallback 模拟复制失败（剪贴板为空 / 注入失败）时，是否回退使用系统默认复制键重试。
	// 开启后：若自定义复制键未生效，自动再用系统原生复制键（macOS Cmd+C / 其他 Ctrl+C）
	// 执行一次，而不是放弃复制。
	Fallback bool `json:"fallback" mapstructure:"fallback"`
}

// ExecKeyConfig 执行类快捷键：程序主动用 robotgo 模拟按下这些键来完成动作，
// 不参与 mgr.Register 注册（它们是"被按下的键"，不是"被监听的键"）。
type ExecKeyConfig struct {
	// Copy 复制键：程序模拟按下它完成系统复制（如 macOS Cmd+C）后读剪贴板翻译。
	Copy ExecKeyEntry `json:"copy" mapstructure:"copy"`
}

// TTSConfig 语音合成配置
type TTSConfig struct {
	// Engine TTS 引擎标识（如 system）。
	Engine string `json:"engine" mapstructure:"engine"`
	// Speed 语速倍率，1.0 为正常语速。
	Speed float64 `json:"speed" mapstructure:"speed"`
}

// HttpLogConfig HTTP 请求日志配置（暂不同步到前端 UI）。
type HttpLogConfig struct {
	// Enabled 是否开启 HTTP 请求日志。
	Enabled bool `json:"enabled" mapstructure:"enabled"`
	// RetentionDays 日志保留天数。
	RetentionDays int `json:"retention_days" mapstructure:"retention_days"`
}

// LogConfig 应用运行日志配置（仅写 settings.json，由 main 的日志模块消费，不直接同步前端 UI）。
// 等级支持 debug / info / warn / error；清理策略基于按天滚动的日志文件。
type LogConfig struct {
	// Level 日志等级：debug / info / warn / error（空串或非法定值回退 info）。
	Level string `json:"level" mapstructure:"level"`
	// RetentionDays 日志文件保留天数（<=0 表示不清理）。
	RetentionDays int `json:"retention_days" mapstructure:"retention_days"`
	// Compress 是否将过期的旧日志文件压缩为 .gz（保留天数内有效）。
	Compress bool `json:"compress" mapstructure:"compress"`
}

// DNSConfig DNS 解析配置
type DNSConfig struct {
	// Enabled 是否启用该 DNS 配置。
	Enabled bool `json:"enabled" mapstructure:"enabled"`
	// Servers DNS 服务器地址列表。
	Servers []string `json:"servers" mapstructure:"servers"`
}

// ProxyConfig 网络代理配置（暂不同步到前端 UI）。
type ProxyConfig struct {
	// Enabled 是否启用代理。
	Enabled bool `json:"enabled" mapstructure:"enabled"`
	// Protocol 代理协议：http / https / socks5。
	Protocol string `json:"protocol" mapstructure:"protocol"`
	// Host 代理主机地址。
	Host string `json:"host" mapstructure:"host"`
	// Port 代理端口。
	Port int `json:"port" mapstructure:"port"`
	// Username 代理认证用户名（可选）。
	Username string `json:"username" mapstructure:"username"`
	// Password 代理认证密码（可选）。
	Password string `json:"password" mapstructure:"password"`
}

// UpdaterConfig 自动更新相关配置（仅 Prerelease / Source 这类用户可决策项；
// token / provider / 资源匹配规则由 main.go 代码固定，不在此配置）。
type UpdaterConfig struct {
	// Prerelease 是否允许检测预发布版（pre-release）更新。
	// 开启后：检查更新时会把 GitHub 仓库的 pre-release 版本也纳入候选。
	Prerelease bool `json:"prerelease" mapstructure:"prerelease"`
	// Source 指定更新检测源：空 / "github" / "cnb"。
	//   - 空（默认）：沿用当前逻辑，按界面语言自动选源（英文走 GitHub，中文走 CNB）。
	//   - "github"：强制只走官方 GitHub（含 SHA256SUMS 校验）。
	//   - "cnb"：强制只走 CNB 镜像（需 cnbToken，匿名 401 / 网络不可达即视为「无更新」）。
	// 注意：仅影响「检测 / 下载源」选择，不影响 updater 自身的安装行为。
	Source string `json:"source" mapstructure:"source"`
}

// 更新源取值常量（与 UpdaterConfig.Source 对应）。
const (
	// UpdaterSourceGitHub 强制使用官方 GitHub 源。
	UpdaterSourceGitHub = "github"
	// UpdaterSourceCNB 强制使用 CNB 镜像源。
	UpdaterSourceCNB = "cnb"
)

// DefaultSettings 返回默认设置指针
func DefaultSettings() *Settings {
	return &Settings{
		Language:    string(model.LocaleAuto),
		Theme:       string(model.ThemeAuto),
		DefaultTo:   string(model.ZH),
		DefaultFrom: string(model.Auto),
		Hotkeys: RegisteredHotkeyConfig{
			// Input 唤起主窗口：默认按键 Alt+A 并启用。
			// Screenshot：默认关闭，由用户在设置页开启并绑定按键。
			Input:      HotkeyEntry{Key: "Alt+A", Enabled: true},
			Screenshot: HotkeyEntry{Key: "Alt+S", Enabled: false},
		},
		ExecKeys: ExecKeyConfig{
			Copy: ExecKeyEntry{Key: defaultCopyHotkey(), Enabled: true, Fallback: false},
		},
		TTS:     TTSConfig{Engine: "system", Speed: 1.0},
		HttpLog: HttpLogConfig{Enabled: true, RetentionDays: 30},
		Log:     LogConfig{Level: "info", RetentionDays: 30, Compress: true},
		Proxy:   ProxyConfig{Enabled: false, Protocol: "http", Port: 8080},
		Updater: UpdaterConfig{Prerelease: true},
	}
}

// defaultCopyHotkey 返回复制键的默认值，按系统区分：
// macOS 默认 Cmd+C（系统原生复制键），其他平台默认 Ctrl+C。
// 区分系统的逻辑放在配置文件（这里），代码执行层只按用户配置 (ExecKeys.Copy) 解析模拟，不写死平台键。
func defaultCopyHotkey() string {
	if runtime.GOOS == "darwin" {
		return "Cmd+C"
	}
	return "Ctrl+C"
}

// OnChangeFunc 配置文件变更回调（热重载）
type OnChangeFunc func(newCfg *Settings)

// Service 配置服务（参考 certflow 的 viper Service 模式）
type Service struct {
	mu       sync.RWMutex
	cfg      *Settings
	v        *viper.Viper
	filePath string
	onChange OnChangeFunc
	saving   bool // 标记正在保存，避免触发自身回调
}

// NewService 创建配置服务：建目录、读/写默认、监听文件变更。
func NewService(dataDir string) (*Service, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir failed: %w", err)
	}
	filePath := filepath.Join(dataDir, "settings.json")

	v := viper.New()
	v.SetConfigFile(filePath)
	v.SetConfigType("json")

	s := &Service{
		filePath: filePath,
		cfg:      DefaultSettings(),
		v:        v,
	}

	s.setDefaults()

	if err := v.ReadInConfig(); err != nil {
		// 首次运行：写默认配置
		if os.IsNotExist(err) {
			if err := s.writeConfig(); err != nil {
				return nil, fmt.Errorf("write default config failed: %w", err)
			}
		} else {
			return nil, fmt.Errorf("load config failed: %w", err)
		}
	}

	if err := v.Unmarshal(s.cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config failed: %w", err)
	}
	s.cfg.Path = filePath

	// 启动后重新保存，确保磁盘配置与内存一致（缺失的默认值补回、多余的由写盘覆盖）
	if err := s.writeConfig(); err != nil {
		return nil, fmt.Errorf("write config failed: %w", err)
	}

	s.startWatching()
	return s, nil
}

// setDefaults 把默认值写入 viper（字段缺失时回退）
func (s *Service) setDefaults() {
	def := DefaultSettings()
	s.v.SetDefault("language", def.Language)
	s.v.SetDefault("theme", def.Theme)
	s.v.SetDefault("default_to", def.DefaultTo)
	s.v.SetDefault("default_from", def.DefaultFrom)
	s.v.SetDefault("hotkeys", def.Hotkeys)
	s.v.SetDefault("execkeys", def.ExecKeys)
	s.v.SetDefault("tts", def.TTS)
	s.v.SetDefault("http_log", def.HttpLog)
	s.v.SetDefault("log", def.Log)
	s.v.SetDefault("proxy", def.Proxy)
	s.v.SetDefault("updater", def.Updater)
}

// startWatching 监听配置文件变更（防抖 500ms）
func (s *Service) startWatching() {
	var (
		debounceTimer *time.Timer
		timerMu       sync.Mutex
	)

	s.v.OnConfigChange(func(e fsnotify.Event) {
		s.mu.Lock()
		if s.saving {
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()

		timerMu.Lock()
		if debounceTimer != nil {
			debounceTimer.Stop()
		}
		debounceTimer = time.AfterFunc(500*time.Millisecond, func() {
			s.mu.Lock()
			defer s.mu.Unlock()

			if err := s.v.Unmarshal(s.cfg); err != nil {
				slog.Default().Error("配置热重载失败", slog.Any("error", err))
				return
			}
			s.cfg.Path = s.filePath

			if s.onChange != nil {
				cb := s.onChange
				cfg := s.cfg
				go cb(cfg)
			}
		})
		timerMu.Unlock()
	})

	s.v.WatchConfig()
}

// OnChange 注册配置变更回调
func (s *Service) OnChange(fn OnChangeFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onChange = fn
}

// writeConfig 用全新 viper 实例只写当前 cfg 的字段，写回配置文件，
// 从而清理掉已废弃、残留在文件里的旧 key（避免 v.WriteConfig 把旧 key 一并写回）。
// 标记 saving 防自触发热重载。
func (s *Service) writeConfig() error {
	s.saving = true
	defer func() { s.saving = false }()

	w := viper.New()
	w.SetConfigType("json")
	w.Set("language", s.cfg.Language)
	w.Set("theme", s.cfg.Theme)
	w.Set("default_to", s.cfg.DefaultTo)
	w.Set("default_from", s.cfg.DefaultFrom)
	w.Set("hotkeys", s.cfg.Hotkeys)
	w.Set("execkeys", s.cfg.ExecKeys)
	w.Set("tts", s.cfg.TTS)
	w.Set("http_log", s.cfg.HttpLog)
	w.Set("log", s.cfg.Log)
	w.Set("proxy", s.cfg.Proxy)
	w.Set("updater", s.cfg.Updater)

	return w.WriteConfigAs(s.filePath)
}

// Save 持久化配置
func (s *Service) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeConfig()
}

// Get 返回当前配置（调用方只读，勿修改返回的指针内容）
func (s *Service) Get() *Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}
