package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func TestOpenAlexConfigAndEnv(t *testing.T) {
	t.Setenv("WEBSEARCH_CONFIG", "")
	t.Setenv("WEBSEARCH_DASHBOARD_CONFIG", "")
	for _, tc := range []struct {
		name string
		yaml string
		env  string
		want string
	}{
		{"anonymous", "mode: engine\n", "", ""},
		{"yaml", "academic:\n  openalex_api_key: yaml-test-key\n", "", "yaml-test-key"},
		{"env-override", "academic:\n  openalex_api_key: yaml-test-key\n", "env-test-key", "env-test-key"},
		{"env-without-yaml-field", "mode: engine\n", "env-test-key", "env-test-key"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)
			t.Setenv("OPENALEX_API_KEY", tc.env)
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(tc.yaml), 0600); err != nil {
				t.Fatal(err)
			}
			conf, err := Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if conf.Academic.OpenAlexAPIKey != tc.want {
				t.Fatal("OpenAlex key was not parsed or overridden correctly")
			}
			if Default().Academic.OpenAlexAPIKey != tc.env {
				t.Fatal("default config did not apply OpenAlex environment key")
			}
		})
	}
}
