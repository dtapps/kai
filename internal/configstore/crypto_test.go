package configstore

import (
	"strings"
	"testing"
)

func TestEncryptDecryptRoundtrip(t *testing.T) {
	cases := []string{"", "sk-1234567890abcdef", "appid|secret-key", "包含中文的密钥/特殊字符!@#"}
	for _, plain := range cases {
		enc, err := EncryptSecret(plain)
		if err != nil {
			t.Fatalf("encrypt %q: %v", plain, err)
		}
		if plain == "" {
			if enc != "" {
				t.Fatalf("空串应返回空，得到 %q", enc)
			}
			continue
		}
		if enc == plain {
			t.Fatalf("密文不应等于明文: %q", enc)
		}
		if !strings.HasPrefix(enc, cipherPrefix) {
			t.Fatalf("密文应带前缀: %q", enc)
		}
		dec, err := DecryptSecret(enc)
		if err != nil {
			t.Fatalf("decrypt: %v", err)
		}
		if dec != plain {
			t.Fatalf("往返不一致: 期望 %q 得到 %q", plain, dec)
		}
	}
}

func TestDecryptLegacyPlaintext(t *testing.T) {
	// 旧版明文数据（无前缀）应原样返回，保证迁移前数据可读。
	plain := "old-plain-secret"
	dec, err := DecryptSecret(plain)
	if err != nil {
		t.Fatalf("decrypt legacy: %v", err)
	}
	if dec != plain {
		t.Fatalf("legacy 应原样返回，得到 %q", dec)
	}
}

func TestDeriveKeyStable(t *testing.T) {
	k1, err := deriveKey()
	if err != nil {
		t.Fatalf("deriveKey: %v", err)
	}
	k2, err := deriveKey()
	if err != nil {
		t.Fatalf("deriveKey: %v", err)
	}
	if len(k1) != 32 {
		t.Fatalf("密钥长度应为 32，得到 %d", len(k1))
	}
	for i := range k1 {
		if k1[i] != k2[i] {
			t.Fatalf("同一设备派生密钥应稳定")
		}
	}
}
