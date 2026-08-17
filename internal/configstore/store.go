// Package configstore 提供应用配置类数据的 SQLite 持久化。
// 数据库查询代码由 sqlc 从 query.sql 自动生成；本文件提供 DB 生命周期管理
// 和引擎配置到数据库模型的类型转换。
package configstore

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"strings"
	"time"

	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/sqlite"
)

//go:embed schema.sql
var schemaSQL string

// Store 封装 config.db 的数据库连接与生成查询。
type Store struct {
	db *sql.DB
	*Queries
}

// Open 打开（或创建）SQLite 配置数据库并执行迁移。
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite3", sqlite.BuildDSN(path))
	if err != nil {
		return nil, fmt.Errorf(i18n.T("err.configstore_open_db"), err, err)
	}
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf(i18n.T("err.configstore_pragma"), p, err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, schemaSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf(i18n.T("err.configstore_migrate"), err, err)
	}
	s := &Store{db: db, Queries: New(db)}
	// 一次性迁移：把历史明文凭据加密（无前缀的旧数据），已加密（带前缀）的跳过。
	if err := s.MigrateSecrets(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf(i18n.T("err.configstore_migrate_secrets"), err, err)
	}
	// 一次性迁移：引擎标识 system → apple（命名统一，与 vision 对称）。
	// 旧库 engines 表中 engine='system' 的行需更新为 'apple'，幂等可重复调用。
	if err := s.MigrateEngineNames(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf(i18n.T("err.configstore_migrate_engine_names"), err, err)
	}
	return s, nil
}

// MigrateEngineNames 将存量 engines 表中 engine='system' 的行改名为 'apple'。
// 历史版本用 "system" 作为 macOS 系统翻译引擎标识，现统一为 "apple"（与 "vision" 对称）。
// 幂等：对已是 'apple' 的库无任何影响。
func (s *Store) MigrateEngineNames(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := s.RenameEngineByName(ctx, RenameEngineByNameParams{
		Engine:   "apple",  // 新标识（SET engine = ?）
		Engine_2: "system", // 旧标识（WHERE engine = ?）
	}); err != nil {
		return fmt.Errorf(i18n.T("err.configstore_rename_engine"), err, err)
	}
	return nil
}

// MigrateSecrets 将 engines 表中仍是明文的 api_key/secret 加密写回，
// 已加密（带 cipherPrefix）的记录保持不变。幂等，可重复调用。
func (s *Store) MigrateSecrets(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	rows, err := s.LoadEngines(ctx)
	if err != nil {
		return err
	}
	for _, r := range rows {
		needUpdate := false
		apiKey := r.ApiKey
		secret := r.Secret
		if r.ApiKey != "" && !strings.HasPrefix(r.ApiKey, cipherPrefix) {
			if enc, e := EncryptSecret(r.ApiKey); e == nil {
				apiKey = enc
				needUpdate = true
			}
		}
		if r.Secret != "" && !strings.HasPrefix(r.Secret, cipherPrefix) {
			if enc, e := EncryptSecret(r.Secret); e == nil {
				secret = enc
				needUpdate = true
			}
		}
		if !needUpdate {
			continue
		}
		if err := s.UpdateEngineByID(ctx, UpdateEngineByIDParams{
			Engine:   r.Engine,
			Enabled:  r.Enabled,
			ApiKey:   apiKey,
			Secret:   secret,
			Extra:    r.Extra,
			Endpoint: r.Endpoint,
			ID:       r.ID,
		}); err != nil {
			return err
		}
	}
	return nil
}

// Close 关闭数据库连接。
func (s *Store) Close() error {
	return s.db.Close()
}

// ---- 单条引擎操作（替代全量 SaveEngineConfigs） ----

// InsertEngineConfig 新增一个引擎配置，返回 SQLite 分配的自增 ID。
// api_key / secret 在落库前加密（敏感字段不明文存储）。
func (s *Store) InsertEngineConfig(ctx context.Context, e *engine.EngineConfig) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	apiKey, err := EncryptSecret(e.APIKey)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", i18n.T("err.configstore_encrypt_apikey"), err)
	}
	secret, err := EncryptSecret(e.Secret)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", i18n.T("err.configstore_encrypt_secret"), err)
	}
	id, err := s.InsertEngine(ctx, InsertEngineParams{
		Engine:   e.Engine,
		Enabled:  boolToInt64(e.Enabled),
		ApiKey:   apiKey,
		Secret:   secret,
		Extra:    e.Extra,
		Endpoint: e.Endpoint,
	})
	if err != nil {
		return 0, fmt.Errorf(i18n.T("err.configstore_insert_engine"), e.Engine, err, e.Engine, err)
	}
	e.ID = id
	return id, nil
}

// UpdateEngineConfig 按 ID 更新单个引擎的全部字段。
// api_key / secret 在落库前加密。
func (s *Store) UpdateEngineConfig(ctx context.Context, e *engine.EngineConfig) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	apiKey, err := EncryptSecret(e.APIKey)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("err.configstore_encrypt_apikey"), err)
	}
	secret, err := EncryptSecret(e.Secret)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("err.configstore_encrypt_secret"), err)
	}
	return s.UpdateEngineByID(ctx, UpdateEngineByIDParams{
		Engine:   e.Engine,
		Enabled:  boolToInt64(e.Enabled),
		ApiKey:   apiKey,
		Secret:   secret,
		Extra:    e.Extra,
		Endpoint: e.Endpoint,
		ID:       e.ID,
	})
}

// SetEngineEnabled 按 ID 切换单个引擎的启用/禁用状态。
func (s *Store) SetEngineEnabled(ctx context.Context, id int64, enabled bool) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return s.UpdateEngineEnabled(ctx, UpdateEngineEnabledParams{
		ID:      id,
		Enabled: boolToInt64(enabled),
	})
}

// DeleteEngineByID 按 ID 删除单个引擎。
func (s *Store) DeleteEngineByID(ctx context.Context, id int64) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return s.Queries.DeleteEngineByID(ctx, id)
}

// GetEngineByName 从数据库按引擎名查询单个引擎配置，未找到返回 (nil, nil)。
func (s *Store) GetEngineByName(ctx context.Context, name string) (*engine.EngineConfig, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	row, err := s.Queries.GetEngineByName(ctx, name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf(i18n.T("err.configstore_get_engine_name"), err, err)
	}
	return engineConfigFromDB(row), nil
}

// GetEngineByID 从数据库按 ID 查询单个引擎配置，未找到返回 (nil, nil)。
func (s *Store) GetEngineByID(ctx context.Context, id int64) (*engine.EngineConfig, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	row, err := s.Queries.GetEngineByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf(i18n.T("err.configstore_get_engine_id"), err, err)
	}
	return engineConfigFromDB(row), nil
}

// engineConfigFromDB 将 sqlc 生成的 Engine 模型转换为业务层 EngineConfig。
// api_key / secret 从库里的密文解密为明文（供引擎调用与前端回填表单）。
func engineConfigFromDB(e Engine) *engine.EngineConfig {
	apiKey, _ := DecryptSecret(e.ApiKey)
	secret, _ := DecryptSecret(e.Secret)
	return &engine.EngineConfig{
		ID:       e.ID,
		Engine:   e.Engine,
		Enabled:  e.Enabled != 0,
		APIKey:   apiKey,
		Secret:   secret,
		Extra:    e.Extra,
		Endpoint: e.Endpoint,
	}
}

// ---- 初始化 ----

// InitDefaultEngines 在 config.db 为空时写入默认引擎清单。
func (s *Store) InitDefaultEngines(ctx context.Context, defaults []*engine.EngineConfig) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for _, e := range defaults {
		if _, err := s.InsertEngineConfig(ctx, e); err != nil {
			return err
		}
	}
	return nil
}

// ---- 类型转换 & 查找 ----

// EnginesToConfig 将数据库 Engine 模型转换为业务层 engine.EngineConfig。
// api_key / secret 从库里的密文解密为明文。
func EnginesToConfig(rows []Engine) []*engine.EngineConfig {
	out := make([]*engine.EngineConfig, 0, len(rows))
	for _, r := range rows {
		apiKey, _ := DecryptSecret(r.ApiKey)
		secret, _ := DecryptSecret(r.Secret)
		out = append(out, &engine.EngineConfig{
			ID:       r.ID,
			Engine:   r.Engine,
			Enabled:  r.Enabled != 0,
			APIKey:   apiKey,
			Secret:   secret,
			Extra:    r.Extra,
			Endpoint: r.Endpoint,
		})
	}
	return out
}

// EngineIDByName 从引擎配置列表中按名称查找 ID（保留以备内存场景）。
func EngineIDByName(engs []*engine.EngineConfig, name string) int64 {
	for _, e := range engs {
		if e.Engine == name {
			return e.ID
		}
	}
	return 0
}

// EngineNameByID 从引擎配置列表中按 ID 查找名称（保留以备内存场景）。
func EngineNameByID(engs []*engine.EngineConfig, id int64) string {
	for _, e := range engs {
		if e.ID == id {
			return e.Engine
		}
	}
	return ""
}

func boolToInt64(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
