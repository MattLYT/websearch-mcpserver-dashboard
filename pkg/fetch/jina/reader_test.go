package jina

import "testing"

// TestIsPrivateIPString 验证内网判断不再误伤公网段（T14 P3-5）。
func TestIsPrivateIPString(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"172.32.0.1", false},     // 公网段（172.32.0.0/11），不得误伤
		{"172.32.0.0", false},     // 公网段起始
		{"172.16.0.1", true},      // RFC1918
		{"172.31.255.255", true},  // RFC1918 边界
		{"192.168.1.1", true},     // RFC1918
		{"10.0.0.1", true},        // RFC1918
		{"127.0.0.1", true},       // 回环
		{"::1", true},             // 回环
		{"8.8.8.8", false},        // 公网
		{"169.254.169.254", true}, // 链路本地（云元数据）
		{"0.0.0.0", true},         // 未指定
		{"example.com", false},    // 非 IP
		{"", false},
	}
	for _, tt := range tests {
		if got := isPrivateIPString(tt.host); got != tt.want {
			t.Errorf("isPrivateIPString(%q) = %v, want %v", tt.host, got, tt.want)
		}
	}
}

// TestIsPrivateHost 验证 hostname 判定（IP 字面量走本地解析，不触网）。
func TestIsPrivateHost(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"172.32.0.1", false},
		{"172.16.0.1", true},
		{"192.168.1.1", true},
		{"localhost", true},
		{"127.0.0.1", true},
		{"::1", true},
		{"0.0.0.0", true},
		{"169.254.169.254", true},
		{"8.8.8.8", false},
	}
	for _, tt := range tests {
		if got := isPrivateHost(tt.host); got != tt.want {
			t.Errorf("isPrivateHost(%q) = %v, want %v", tt.host, got, tt.want)
		}
	}
}
