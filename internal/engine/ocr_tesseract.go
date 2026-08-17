package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"
)

var (
	// ErrEmptyImage 图片数据为空
	ErrEmptyImage = errors.New(i18n.T("err.ocr_empty_image"))
	// ErrNoScreenshot 当前平台不支持系统截图
	ErrNoScreenshot = errors.New(i18n.T("err.ocr_screenshot_unsupported"))
	// ErrTesseractNotFound 本机未找到 tesseract 可执行文件
	ErrTesseractNotFound = errors.New(i18n.T("err.ocr_tesseract_not_found"))
)

// tesseractCandidates GUI 应用（如打包后的 Kai.app）从 launchd 启动不继承 shell 的 PATH，
// 故额外探测常见安装路径（Homebrew Apple Silicon / Intel）。
var tesseractCandidates = []string{
	"/opt/homebrew/bin/tesseract",
	"/usr/local/bin/tesseract",
	"/usr/bin/tesseract",
}

// resolveTesseract 返回可用的 tesseract 可执行路径；找不到返回空串。
func resolveTesseract() string {
	if p, err := exec.LookPath("tesseract"); err == nil {
		return p
	}
	for _, c := range tesseractCandidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

// TesseractOCR 基于系统 tesseract 命令的本地 OCR 引擎（纯 Go exec 调用，无 CGO）。
// 需要本机安装 tesseract（mac: brew install tesseract；linux: apt install tesseract-ocr）。
// 持有所属引擎配置，OCR 专属参数（langs / timeout）统一从 Extra(JSON) 读取，
// 与 vision 共用 parseOCRExtra 解析，保证 extra 格式一致。
type TesseractOCR struct {
	name   string
	config *EngineConfig // 持有所属引擎配置，从 Extra(JSON) 读取 langs / timeout
	bin    string        // tesseract 可执行路径（用户指定或自动探测）
}

// TesseractStatus 描述本机 tesseract 的安装探测结果，供前端按系统类型展示安装状态。
type TesseractStatus struct {
	Installed bool   `json:"installed"` // 是否探测到 tesseract 可执行文件
	Path      string `json:"path"`      // 探测到的可执行路径（未安装则为空）
	Version   string `json:"version"`   // 探测到的版本号（未安装则为空），取自 `tesseract --version`
	OS        string `json:"os"`        // 当前运行系统（darwin/linux/windows），供前端选择对应安装命令
}

// TesseractInstalled 探测本机是否已安装 tesseract，返回路径、版本与系统类型。
// 与 NewTesseractOCR 的探测逻辑一致（PATH + 常见安装路径）；
// 命中后追加执行 `tesseract --version` 提取版本号（首行形如 tesseract 5.3.4）。
func TesseractInstalled() TesseractStatus {
	status := TesseractStatus{OS: runtime.GOOS}
	if p := resolveTesseract(); p != "" {
		status.Installed = true
		status.Path = p
		status.Version = tesseractVersion(p)
	}
	return status
}

// tesseractVersion 执行 `tesseract --version` 提取版本号（首行形如 "tesseract 5.3.4"）。
// 解析失败返回空串（不影响「已安装」判定）。
func tesseractVersion(bin string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "--version").Output()
	if err != nil {
		return ""
	}
	// 首行示例：tesseract 5.3.4  leptonica-1.83.0  ...
	first, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	fields := strings.FieldsSeq(first)
	for f := range fields {
		// 版本号形如 5.3.4（含点、纯数字段）
		if strings.Count(f, ".") >= 1 && !strings.ContainsAny(f, " /\\") {
			return f
		}
	}
	return ""
}

// NewTesseractOCR 构造 OCR 引擎。cfg 为 tesseract 引擎的 EngineConfig（含 Extra(JSON) 中的
// langs / timeout_sec）；bin(Endpoint) 为空时自动探测本机 tesseract。
func NewTesseractOCR(cfg *EngineConfig) *TesseractOCR {
	bin := ""
	if cfg != nil {
		bin = cfg.Endpoint
	}
	if bin == "" {
		bin = resolveTesseract()
	}
	return &TesseractOCR{name: "tesseract", config: cfg, bin: bin}
}

// ocrOptions 解析当前配置与本次请求的参数，得到最终生效的 langs / timeout。
// 优先级：req 显式覆盖 > 引擎 Extra 配置 > 内置默认(chi_sim+eng / 60s)。
func (t *TesseractOCR) ocrOptions(req model.OcrRequest) (langs string, timeoutSec int) {
	langs = DefaultOCRLangs["tesseract"]
	timeoutSec = DefaultOCRTimeoutSec
	e := parseOCRExtra(t.name, optExtra(t.config))
	if e.Langs != "" {
		langs = e.Langs
	}
	if e.TimeoutSec > 0 {
		timeoutSec = e.TimeoutSec
	}
	if req.TimeoutSec > 0 {
		timeoutSec = req.TimeoutSec
	}
	return
}

// optExtra 安全取 EngineConfig.Extra；cfg 为 nil 时返回空串。
func optExtra(cfg *EngineConfig) string {
	if cfg == nil {
		return ""
	}
	return cfg.Extra
}

// Name 引擎名
func (t *TesseractOCR) Name() string { return t.name }

// Recognize 识别图片字节中的文字
func (t *TesseractOCR) Recognize(ctx context.Context, req model.OcrRequest) (*model.OcrResult, error) {
	if len(req.ImageData) == 0 {
		return nil, ErrEmptyImage
	}
	langs, timeoutSec := t.ocrOptions(req)

	// 以 req 携带的 ctx 为主，叠加引擎配置的超时上限（含 Swift/vision 约定的 +10s 余量）。
	if _, ok := ctx.Deadline(); !ok && timeoutSec > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutSec+10)*time.Second)
		defer cancel()
	}

	tmp, err := os.CreateTemp("", "kai-ocr-*.png")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(req.ImageData); err != nil {
		return nil, err
	}
	tmp.Close()

	outBase := strings.TrimSuffix(tmp.Name(), ".png")
	bin := t.bin
	if bin == "" {
		bin = "tesseract" // 兜底，便于在错误中暴露 PATH 问题
	}
	cmd := exec.CommandContext(ctx, bin, tmp.Name(), outBase, "-l", langs, "--psm", "6")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		var msg string
		if t.bin == "" {
			msg = ErrTesseractNotFound.Error()
		} else {
			msg = fmt.Sprintf("%s(%v): %s", i18n.T("err.ocr_tesseract_exec_failed"), err, stderr.String())
		}
		return nil, &OcrError{Msg: msg}
	}

	txtPath := outBase + ".txt"
	raw, err := os.ReadFile(txtPath)
	if err != nil {
		return nil, err
	}
	_ = os.Remove(txtPath)

	text := strings.TrimSpace(string(raw))
	regions := []model.OcrRegion{{Text: text, Conf: 0, Box: nil}}
	return &model.OcrResult{
		Engine:  t.name,
		Text:    text,
		Regions: regions,
	}, nil
}

// OcrError OCR 执行错误
type OcrError struct{ Msg string }

func (e *OcrError) Error() string { return e.Msg }

// CaptureRegion 弹出系统交互式选区截图（用户拖拽框选），返回裁剪后 PNG 字节。
// 该模式会让用户用鼠标在屏幕上拖出一个矩形，松手后落盘到临时文件；
// 需屏幕录制授权。依赖 macOS 自带 screencapture（无需安装）。
func CaptureRegion(ctx context.Context) ([]byte, error) {
	switch runtime.GOOS {
	case "darwin":
		tmp, err := os.CreateTemp("", "kai-region-*.png")
		if err != nil {
			return nil, err
		}
		path := tmp.Name()
		tmp.Close()
		defer os.Remove(path)
		// -i 交互模式（用户拖拽框选选区），-x 静默无快门声。
		// 注意：-R 是「按指定矩形(非交互)捕获」需带 -R x,y,w,h 值，不能和 -i 混用，
		// 单独 "-i -R" 会让 screencapture 直接报 exit status 1。交互框选只用 -i。
		cmd := exec.CommandContext(ctx, "screencapture", "-i", "-x", path)
		if err := cmd.Run(); err != nil {
			// 用户按 ESC 取消框选时 screencapture 退出码非 0；视为取消，不视为系统错误。
			return nil, fmt.Errorf("%s: %w", i18n.T("err.ocr_region_capture_failed"), err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", i18n.T("err.ocr_read_region_failed"), err)
		}
		if len(data) == 0 {
			// screencapture 退出码 0 但产出空文件（如框选面积为 0），返回明确错误避免下游 OCR 误报。
			return nil, ErrEmptyImage
		}
		return data, nil
	default:
		return nil, ErrNoScreenshot
	}
}

// CaptureScreenshot 用系统截图工具截全屏，返回 PNG 字节。
func CaptureScreenshot() ([]byte, error) {
	tmp, err := os.CreateTemp("", "kai-shot-*.png")
	if err != nil {
		return nil, err
	}
	path := tmp.Name()
	tmp.Close()
	defer os.Remove(path)

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("screencapture", "-x", path)
	case "linux":
		// ImageMagick import 截取全屏
		cmd = exec.Command("import", "-window", "root", path)
	case "windows":
		// TODO(M6): 用内置工具，如 powershell 调 ScreenCapture
		return nil, ErrNoScreenshot
	default:
		return nil, ErrNoScreenshot
	}
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}
