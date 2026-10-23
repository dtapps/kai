package buildinfo

import (
	"path/filepath"
	"testing"
)

// setDev 临时修改 Dev 变量（包级可变状态），测试结束自动还原。
func setDev(t *testing.T, v string) {
	t.Helper()
	old := Dev
	Dev = v
	t.Cleanup(func() { Dev = old })
}

func TestIsDev(t *testing.T) {
	cases := []struct {
		name string
		dev  string
		want bool
	}{
		{"字符串 true", "true", true},
		{"数字 1", "1", true},
		{"字符串 false", "false", false},
		{"空串（未注入）", "", false},
		{"大写 TRUE（严格匹配不认）", "TRUE", false},
		{"其它值", "yes", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			setDev(t, c.dev)
			if got := IsDev(); got != c.want {
				t.Fatalf("IsDev() = %v, want %v (Dev=%q)", got, c.want, c.dev)
			}
		})
	}
}

func TestDataHome(t *testing.T) {
	setDev(t, "true")
	if got := DataHome(); got != ".kai.dev" {
		t.Fatalf("Dev=true 时 DataHome() = %q, want %q", got, ".kai.dev")
	}

	setDev(t, "false")
	if got := DataHome(); got != ".kai" {
		t.Fatalf("Dev=false 时 DataHome() = %q, want %q", got, ".kai")
	}
}

func TestDirsAreUnderHome(t *testing.T) {
	home := filepath.Join(string(filepath.Separator), "home", "tester")

	t.Run("发布模式", func(t *testing.T) {
		setDev(t, "false")
		wantData := filepath.Join(home, ".kai")
		if got := DataDir(home); got != wantData {
			t.Fatalf("DataDir() = %q, want %q", got, wantData)
		}
		if got, want := DBDir(home), filepath.Join(wantData, "data"); got != want {
			t.Fatalf("DBDir() = %q, want %q", got, want)
		}
		if got, want := LogDir(home), filepath.Join(wantData, "logs"); got != want {
			t.Fatalf("LogDir() = %q, want %q", got, want)
		}
	})

	t.Run("开发模式", func(t *testing.T) {
		setDev(t, "1")
		wantData := filepath.Join(home, ".kai.dev")
		if got := DataDir(home); got != wantData {
			t.Fatalf("DataDir() = %q, want %q", got, wantData)
		}
		if got, want := DBDir(home), filepath.Join(wantData, "data"); got != want {
			t.Fatalf("DBDir() = %q, want %q", got, want)
		}
		if got, want := LogDir(home), filepath.Join(wantData, "logs"); got != want {
			t.Fatalf("LogDir() = %q, want %q", got, want)
		}
	})
}

// 数据库与日志都必须在数据根目录之下，否则卸载/清理会漏文件。
func TestDBAndLogDirAreUnderDataDir(t *testing.T) {
	setDev(t, "false")
	home := t.TempDir()
	data := DataDir(home)

	for name, dir := range map[string]string{"DBDir": DBDir(home), "LogDir": LogDir(home)} {
		rel, err := filepath.Rel(data, dir)
		if err != nil {
			t.Fatalf("%s: filepath.Rel 失败: %v", name, err)
		}
		if rel == ".." || rel == "." {
			t.Fatalf("%s() = %q 不在 DataDir() = %q 之下", name, dir, data)
		}
	}
}
