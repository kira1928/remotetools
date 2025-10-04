package config

import (
	"testing"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		v1       string
		v2       string
		expected int
	}{
		{"8.0.5", "8.0.4", 1},
		{"8.0.4", "8.0.5", -1},
		{"8.0.5", "8.0.5", 0},
		{"8.1.0", "8.0.9", 1},
		{"7.9.9", "8.0.0", -1},
		{"10.0.0", "9.9.9", 1},
		{"1.2.3", "1.2", 1},
		{"1.2", "1.2.0", 0},
		{"2.0", "1.9.9", 1},
		// v 前缀兼容
		{"v1.2.3", "1.2.3", 0},
		{"v2.0.0", "v1.9.9", 1},
		{"1.2", "v1.2.0", 0},
		// 预发布版本遵循 semver 规则：先行版本 < 正式发布
		{"v1.0.0-rc.1", "v1.0.0", -1},
		{"v1.0.0-alpha", "v1.0.0-beta", -1},
		// 构建元数据不影响排序
		{"v1.0.0+build.1", "v1.0.0+build.2", 0},
	}

	for _, tt := range tests {
		result := compareVersions(tt.v1, tt.v2)
		if result != tt.expected {
			t.Errorf("compareVersions(%s, %s) = %d; expected %d", tt.v1, tt.v2, result, tt.expected)
		}
	}
}

func TestGetLatestVersion(t *testing.T) {
	tests := []struct {
		versions []string
		expected string
	}{
		{[]string{"8.0.5", "8.0.4", "8.0.3"}, "8.0.5"},
		{[]string{"8.0.4", "8.0.5", "8.0.3"}, "8.0.5"},
		{[]string{"7.0.0", "8.0.0", "9.0.0"}, "9.0.0"},
		{[]string{"1.0.0"}, "1.0.0"},
		{[]string{}, ""},
		{[]string{"10.0.0", "9.9.9", "9.10.0"}, "10.0.0"},
	}

	for _, tt := range tests {
		result := GetLatestVersion(tt.versions)
		if result != tt.expected {
			t.Errorf("GetLatestVersion(%v) = %s; expected %s", tt.versions, result, tt.expected)
		}
	}
}
