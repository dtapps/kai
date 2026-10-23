package configstore

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"cnb.cool/dtapp/kai/internal/engine"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatalf("Open 失败: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// ---------- 加解密 ----------

func TestEncryptDecryptWithPrefix(t *testing.T) {
	plain := "sk-very-secret-value-测试"
	enc, err := EncryptSecret(plain)
	if err != nil {
		t.Fatalf("EncryptSecret 失败: %v", err)
	}
	if enc == plain {
		t.Fatal("密文与明文相同，说明根本没加密")
	}
	if !strings.HasPrefix(enc, cipherPrefix) {
		t.Fatalf("密文缺少前缀 %q: %q", cipherPrefix, enc)
	}

	got, err := DecryptSecret(enc)
	if err != nil {
		t.Fatalf("DecryptSecret 失败: %v", err)
	}
	if got != plain {
		t.Fatalf("解密结果 = %q, want %q", got, plain)
	}
}

// 同一明文两次加密必须不同（随机 nonce），否则泄露「两个字段值相同」。
func TestEncryptIsRandomized(t *testing.T) {
	a, err := EncryptSecret("same")
	if err != nil {
		t.Fatal(err)
	}
	b, err := EncryptSecret("same")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("两次加密结果相同，nonce 未随机化")
	}
}

func TestEncryptEmpty(t *testing.T) {
	got, err := EncryptSecret("")
	if err != nil {
		t.Fatalf("空串不应报错: %v", err)
	}
	if got != "" {
		t.Fatalf("空串应返回空串，实际 %q", got)
	}
}

// 历史明文数据没有前缀，解密时必须原样返回（向后兼容）。
func TestDecryptPlaintextPassthrough(t *testing.T) {
	if got, err := DecryptSecret("legacy-plain-key"); err != nil || got != "legacy-plain-key" {
		t.Fatalf("明文应原样返回，got %q err %v", got, err)
	}
	if got, err := DecryptSecret(""); err != nil || got != "" {
		t.Fatalf("空串应原样返回，got %q err %v", got, err)
	}
}

func TestDecryptMalformed(t *testing.T) {
	cases := []string{
		cipherPrefix,                 // 只有前缀
		cipherPrefix + "!!!不是base64", // 非法 base64
		cipherPrefix + "YWJj",        // 长度不足以容纳 nonce
	}
	for _, in := range cases {
		if _, err := DecryptSecret(in); err == nil {
			t.Fatalf("畸形密文 %q 应报错", in)
		}
	}
}

// 密文被篡改（GCM 认证失败）必须报错，不能返回垃圾明文。
func TestDecryptTamperedFails(t *testing.T) {
	enc, err := EncryptSecret("original")
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte(enc)
	// 改动最后一个字符（base64 载荷尾部）。
	if raw[len(raw)-1] == 'A' {
		raw[len(raw)-1] = 'B'
	} else {
		raw[len(raw)-1] = 'A'
	}
	if _, err := DecryptSecret(string(raw)); err == nil {
		t.Fatal("被篡改的密文应认证失败")
	}
}

// ---------- 生命周期 ----------

func TestOpenTwiceIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.db")

	s1, err := Open(path)
	if err != nil {
		t.Fatalf("首次 Open 失败: %v", err)
	}
	defer s1.Close()

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("二次 Open 失败（schema 应幂等）: %v", err)
	}
	defer s2.Close()
}

func TestOpenUnwritablePath(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	s, err := Open(filepath.Join(blocker, "sub", "config.db"))
	if err == nil {
		s.Close()
		t.Fatal("不可写路径应返回错误")
	}
}

// ---------- 引擎 CRUD ----------

func TestEngineCRUD(t *testing.T) {
	s := openTestStore(t)
	ctx := t.Context()

	in := &engine.EngineConfig{
		Engine: "deepl", Enabled: true,
		APIKey: "app-key", Secret: "app-secret",
		Extra: `{"model":"x"}`, Endpoint: "https://example.com",
	}
	id, err := s.InsertEngineConfig(ctx, in)
	if err != nil {
		t.Fatalf("Insert 失败: %v", err)
	}
	if id <= 0 {
		t.Fatalf("自增 ID = %d", id)
	}
	if in.ID != id {
		t.Fatalf("Insert 未回填 ID: %d", in.ID)
	}

	// 落库的必须是密文。
	var rawKey string
	if err := s.db.QueryRow(`SELECT api_key FROM engines WHERE id = ?`, id).Scan(&rawKey); err != nil {
		t.Fatal(err)
	}
	if rawKey == "app-key" || !strings.HasPrefix(rawKey, cipherPrefix) {
		t.Fatalf("库里存的不是密文: %q", rawKey)
	}

	byName, err := s.GetEngineByName(ctx, "deepl")
	if err != nil || byName == nil {
		t.Fatalf("GetEngineByName 失败: %v, %v", byName, err)
	}
	if byName.APIKey != "app-key" || byName.Secret != "app-secret" {
		t.Fatalf("读取时应解密为明文，实际 %+v", byName)
	}
	if !byName.Enabled || byName.Extra != `{"model":"x"}` || byName.Endpoint != "https://example.com" {
		t.Fatalf("其它字段不符: %+v", byName)
	}

	byID, err := s.GetEngineByID(ctx, id)
	if err != nil || byID == nil || byID.Engine != "deepl" {
		t.Fatalf("GetEngineByID 失败: %+v, %v", byID, err)
	}

	// 更新：换名 + 关闭 + 改密。
	upd := *byName
	upd.Engine = "deepl-free"
	upd.Enabled = false
	upd.APIKey = "new-key"
	if err := s.UpdateEngineConfig(ctx, &upd); err != nil {
		t.Fatalf("Update 失败: %v", err)
	}

	after, err := s.GetEngineByID(ctx, id)
	if err != nil || after == nil {
		t.Fatalf("更新后查询失败: %v", err)
	}
	if after.Engine != "deepl-free" || after.Enabled || after.APIKey != "new-key" {
		t.Fatalf("更新未生效: %+v", after)
	}

	// 单独切换开关。
	if err := s.SetEngineEnabled(ctx, id, true); err != nil {
		t.Fatalf("SetEngineEnabled 失败: %v", err)
	}
	if got, _ := s.GetEngineByID(ctx, id); !got.Enabled {
		t.Fatal("SetEngineEnabled(true) 未生效")
	}

	if err := s.DeleteEngineByID(ctx, id); err != nil {
		t.Fatalf("Delete 失败: %v", err)
	}
	if got, err := s.GetEngineByID(ctx, id); err != nil || got != nil {
		t.Fatalf("删除后应查不到: %+v, %v", got, err)
	}
}

func TestGetEngineMissingReturnsNil(t *testing.T) {
	s := openTestStore(t)

	if got, err := s.GetEngineByName(t.Context(), "nope"); err != nil || got != nil {
		t.Fatalf("按名查不到应返回 (nil,nil)，实际 %+v, %v", got, err)
	}
	if got, err := s.GetEngineByID(t.Context(), 99999); err != nil || got != nil {
		t.Fatalf("按 ID 查不到应返回 (nil,nil)，实际 %+v, %v", got, err)
	}
}

// engine 列有 UNIQUE 约束，重复插入必须报错而不是静默产生两行。
func TestInsertDuplicateEngineName(t *testing.T) {
	s := openTestStore(t)
	ctx := t.Context()

	if _, err := s.InsertEngineConfig(ctx, &engine.EngineConfig{Engine: "google", Enabled: true}); err != nil {
		t.Fatalf("首次插入失败: %v", err)
	}
	if _, err := s.InsertEngineConfig(ctx, &engine.EngineConfig{Engine: "google"}); err == nil {
		t.Fatal("同名引擎重复插入应报错")
	}
}

func TestLoadEngines(t *testing.T) {
	s := openTestStore(t)
	ctx := t.Context()

	if rows, err := s.LoadEngines(ctx); err != nil || len(rows) != 0 {
		t.Fatalf("空库应返回 0 行，实际 %d, %v", len(rows), err)
	}

	for _, name := range []string{"google", "deepl", "baidu"} {
		if _, err := s.InsertEngineConfig(ctx, &engine.EngineConfig{Engine: name, Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}

	rows, err := s.LoadEngines(ctx)
	if err != nil {
		t.Fatalf("LoadEngines 失败: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("行数 = %d, want 3", len(rows))
	}
}

func TestInitDefaultEngines(t *testing.T) {
	s := openTestStore(t)
	ctx := t.Context()

	defaults := []*engine.EngineConfig{
		{Engine: "google", Enabled: true},
		{Engine: "apple", Enabled: true},
	}
	if err := s.InitDefaultEngines(ctx, defaults); err != nil {
		t.Fatalf("InitDefaultEngines 失败: %v", err)
	}

	rows, err := s.LoadEngines(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("应写入 2 行，实际 %d", len(rows))
	}

	// 本身不做「是否为空」判断：调用方必须先查空，否则撞唯一约束。
	if err := s.InitDefaultEngines(ctx, defaults); err == nil {
		t.Fatal("重复调用应因 UNIQUE 约束报错（调用方需先判断空库）")
	}
}

// ---------- 迁移 ----------

// 旧库里的明文凭据在 Open 时会被就地加密。
func TestMigrateSecretsEncryptsLegacyRows(t *testing.T) {
	s := openTestStore(t)
	ctx := t.Context()

	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO engines (engine, enabled, api_key, secret) VALUES (?, 1, ?, ?)`,
		"openai", "plain-key", "plain-secret"); err != nil {
		t.Fatalf("写入明文行失败: %v", err)
	}

	if err := s.MigrateSecrets(ctx); err != nil {
		t.Fatalf("MigrateSecrets 失败: %v", err)
	}

	var apiKey, secret string
	if err := s.db.QueryRowContext(ctx, `SELECT api_key, secret FROM engines WHERE engine = 'openai'`).Scan(&apiKey, &secret); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(apiKey, cipherPrefix) || !strings.HasPrefix(secret, cipherPrefix) {
		t.Fatalf("未加密: apiKey=%q secret=%q", apiKey, secret)
	}

	// 再跑一次必须幂等（不能二次加密导致解不开）。
	if err := s.MigrateSecrets(ctx); err != nil {
		t.Fatalf("二次迁移失败: %v", err)
	}
	got, err := s.GetEngineByName(ctx, "openai")
	if err != nil || got == nil {
		t.Fatalf("查询失败: %v", err)
	}
	if got.APIKey != "plain-key" || got.Secret != "plain-secret" {
		t.Fatalf("二次迁移后解密异常: %+v", got)
	}
}

// 历史标识 system 会被迁移成 apple。
func TestMigrateEngineNames(t *testing.T) {
	s := openTestStore(t)
	ctx := t.Context()

	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO engines (engine, enabled) VALUES ('system', 1)`); err != nil {
		t.Fatalf("写入 system 行失败: %v", err)
	}

	if err := s.MigrateEngineNames(ctx); err != nil {
		t.Fatalf("MigrateEngineNames 失败: %v", err)
	}

	if got, err := s.GetEngineByName(ctx, "system"); err != nil || got != nil {
		t.Fatalf("system 行应已迁移走: %+v, %v", got, err)
	}
	if got, err := s.GetEngineByName(ctx, "apple"); err != nil || got == nil {
		t.Fatalf("应存在 apple 行: %+v, %v", got, err)
	}

	// 幂等。
	if err := s.MigrateEngineNames(ctx); err != nil {
		t.Fatalf("二次迁移失败: %v", err)
	}
}

// apple 与 system 同时存在时，迁移会撞唯一约束——这是已知边界，必须报错而不是静默丢数据。
func TestMigrateEngineNamesConflict(t *testing.T) {
	s := openTestStore(t)
	ctx := t.Context()

	for _, name := range []string{"system", "apple"} {
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO engines (engine, enabled) VALUES (?, 1)`, name); err != nil {
			t.Fatal(err)
		}
	}

	if err := s.MigrateEngineNames(ctx); err == nil {
		t.Fatal("apple 已存在时应因唯一约束报错")
	}
}

// ---------- 纯函数 ----------

func TestEnginesToConfigAndLookups(t *testing.T) {
	rows := []Engine{
		{ID: 7, Engine: "google", Enabled: 1, Extra: "e", Endpoint: "u"},
		{ID: 9, Engine: "deepl", Enabled: 0},
	}
	cfgs := EnginesToConfig(rows)
	if len(cfgs) != 2 {
		t.Fatalf("数量 = %d", len(cfgs))
	}
	if cfgs[0].ID != 7 || !cfgs[0].Enabled || cfgs[1].Enabled {
		t.Fatalf("enabled 转换错误: %+v", cfgs)
	}

	if got := EngineIDByName(cfgs, "deepl"); got != 9 {
		t.Fatalf("EngineIDByName = %d, want 9", got)
	}
	if got := EngineIDByName(cfgs, "missing"); got != 0 {
		t.Fatalf("未命中应返回 0，实际 %d", got)
	}
	if got := EngineNameByID(cfgs, 7); got != "google" {
		t.Fatalf("EngineNameByID = %q", got)
	}
	if got := EngineNameByID(cfgs, 404); got != "" {
		t.Fatalf("未命中应返回空串，实际 %q", got)
	}

	if boolToInt64(true) != 1 || boolToInt64(false) != 0 {
		t.Fatal("boolToInt64 转换错误")
	}
}

var _ = context.Background
