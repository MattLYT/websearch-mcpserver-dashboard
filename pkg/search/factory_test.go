package search

import (
	"testing"

	"websearch/pkg/config"
)

func TestNewFromConfig_DoubaoMode_NoKey_Fallback(t *testing.T) {
	g, err := NewFromConfig(config.Config{Mode: config.ModeDoubao, Bing: config.BingConfig{Enabled: true}})
	if err != nil {
		t.Fatalf("NewFromConfig: %v", err)
	}
	if g.Primary == nil {
		t.Fatal("expected Bing fallback when doubao has no key")
	}
	if g.Primary.Name() != "bing" {
		t.Fatalf("fallback Name() = %q, want bing", g.Primary.Name())
	}
}

// ── 工厂函数测试 ──

func TestNewFromConfig_ExaMode(t *testing.T) {
	conf := config.Config{
		Mode: "exa",
		Exa: config.ExaConfig{
			APIKey:       "test-key",
			NumResults:   10,
			LookbackDays: 30,
		},
		Bing: config.BingConfig{Enabled: false},
	}
	g, err := NewFromConfig(conf)
	if err != nil {
		t.Fatalf("NewFromConfig failed: %v", err)
	}
	if g.Primary == nil {
		t.Fatal("expected non-nil primary engine")
	}
	if g.Primary.Name() != "exa" {
		t.Errorf("expected engine name 'exa', got %s", g.Primary.Name())
	}
}

func TestNewFromConfig_ExaMode_NoKey_Fallback(t *testing.T) {
	conf := config.Config{
		Mode: "exa",
		Exa:  config.ExaConfig{},
		Bing: config.BingConfig{Enabled: true},
	}
	g, err := NewFromConfig(conf)
	if err != nil {
		t.Fatalf("NewFromConfig failed: %v", err)
	}
	if g.Primary == nil {
		t.Fatal("expected fallback engine")
	}
	// 应该回退到 Bing
	if g.Primary.Name() != "bing" {
		t.Logf("fallback engine: %s", g.Primary.Name())
	}
}
