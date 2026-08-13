package configstore

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// 敏感字段（api_key / secret）在落库前用 AES-GCM 加密，读取时解密。
// 设计目标：即使 config.db 文件被单独复制/泄露，也无法还原明文凭据
// （密钥派生自本机设备指纹，不在 db 内、也不进代码仓库）。
//
// 加密后的密文以 "kai:cipher:" 前缀标记，存入 TEXT 列；旧版明文数据无此前缀，
// 解密时按前缀判别——既能兼容历史明文，又能避免把已加密数据二次加密。

const cipherPrefix = "kai:cipher:"

// 固定 salt：让 HKDF 派生稳定且与本应用绑定（非机密，可公开）。
var hkdfSalt = []byte("kai-configstore-aes-key-salt-v1")

// deriveKey 基于设备指纹派生 32 字节 AES-256 密钥（HKDF-SHA256，Extract+Expand）。
// 优先用 macOS 的 IOPlatformUUID（稳定且唯一），其它平台回退到 hostname+machine-id。
// 密钥不落库、不进代码仓库，仅在本机由设备指纹实时派生，因此即便 config.db 单独泄露也无法还原明文。
func deriveKey() ([]byte, error) {
	secret, err := deviceSecret()
	if err != nil {
		return nil, err
	}
	// HKDF-Extract：PRK = HMAC-Hash(salt, secret)
	prk := hmacSHA256(hkdfSalt, secret)
	// HKDF-Expand：OKM = T(1) || T(2) ...，info 固定，输出 32 字节
	const info = "kai-config-key"
	t := make([]byte, 0, 32)
	block := make([]byte, 32)
	var counter byte = 1
	for len(t) < 32 {
		h := hmacSHA256(prk, append(append([]byte{}, byte(counter)), info...))
		copy(block, h)
		t = append(t, block[:]...)
		counter++
	}
	key := make([]byte, 32)
	copy(key, t[:32])
	return key, nil
}

// hmacSHA256 返回 HMAC-SHA256(secret, msg)。
func hmacSHA256(secret, msg []byte) []byte {
	h := hmac.New(sha256.New, secret)
	h.Write(msg)
	return h.Sum(nil)
}

// deviceSecret 返回本机稳定指纹。
func deviceSecret() ([]byte, error) {
	var raw string
	switch runtime.GOOS {
	case "darwin":
		// IOPlatformUUID 在重装系统后仍保持稳定，是理想的设备绑定源。
		out, err := exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice").
			Output()
		if err == nil {
			s := string(out)
			if idx := strings.Index(s, "\"IOPlatformUUID\""); idx >= 0 {
				rest := s[idx:]
				if _, after, ok := strings.Cut(rest, "\""); ok {
					rest2 := after
					if before, _, ok := strings.Cut(rest2, "\""); ok {
						candidate := before
						if candidate != "" {
							raw = candidate
						}
					}
				}
			}
		}
	case "linux":
		if b, err := os.ReadFile("/etc/machine-id"); err == nil {
			raw = strings.TrimSpace(string(b))
		}
	}
	if raw == "" {
		// 回退：hostname（跨平台可用，虽不如 UUID 稳定，但保证不崩溃）。
		if h, err := exec.Command("hostname").Output(); err == nil {
			raw = strings.TrimSpace(string(h))
		}
	}
	if raw == "" {
		return nil, errors.New("无法获取设备指纹")
	}
	return []byte(raw), nil
}

// EncryptSecret 加密敏感字段；空串直接返回空（不加密空值）。
func EncryptSecret(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	key, err := deriveKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return cipherPrefix + base64.StdEncoding.EncodeToString(ct), nil
}

// DecryptSecret 解密敏感字段。
//   - 空串返回空；
//   - 无 cipherPrefix 视为旧版明文，原样返回（兼容迁移前数据）；
//   - 解密失败（数据损坏或设备变更）返回错误，交由调用方记录日志。
func DecryptSecret(stored string) (string, error) {
	if stored == "" {
		return "", nil
	}
	if !strings.HasPrefix(stored, cipherPrefix) {
		// 旧版明文数据：保持兼容，直接返回。
		return stored, nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, cipherPrefix))
	if err != nil {
		return "", fmt.Errorf("密文 base64 解码失败: %w", err)
	}
	key, err := deriveKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	ns := gcm.NonceSize()
	if len(raw) < ns {
		return "", errors.New("密文长度不足")
	}
	nonce, ct := raw[:ns], raw[ns:]
	plain, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("密文解密失败(可能设备指纹变更): %w", err)
	}
	return string(plain), nil
}
