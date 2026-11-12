package wails_updater_providers

import (
	"testing"

	"golang.org/x/mod/semver"
)

// 用框架内部同款比较器（x/mod/semver）演示五种版本号的不同语义。
func TestZZSemverTable(t *testing.T) {
	vs := []string{"1.2.3", "1.2.3-rc.1", "1.2.3-beta.1", "1.2.3-alpha.1", "1.2.3-nightly.20260101"}
	t.Logf("%-26s %-10s %s", "版本", "isValid", "compare vs 1.2.3")
	for _, v := range vs {
		t.Logf("%-26s %-10v %+d", v, semver.IsValid("v"+v), semver.Compare("v"+v, "v1.2.3"))
	}
	t.Logf("--- 通道间相对大小（-1=左小）")
	pairs := [][2]string{
		{"1.2.3-nightly.20260101", "1.2.3-alpha.1"},
		{"1.2.3-nightly.20260101", "1.2.3-beta.1"},
		{"1.2.3-nightly.20260101", "1.2.3-rc.1"},
		{"1.2.3-alpha.1", "1.2.3-beta.1"},
		{"1.2.3-beta.1", "1.2.3-rc.1"},
	}
	for _, p := range pairs {
		t.Logf("%-26s vs %-22s => %+d", p[0], p[1], semver.Compare("v"+p[0], "v"+p[1]))
	}
	t.Logf("--- 现有 isNewer 对照")
	for _, p := range pairs {
		t.Logf("isNewer(%-26s, %-22s) = %v", p[0], p[1], isNewer(p[0], p[1]))
	}
}
