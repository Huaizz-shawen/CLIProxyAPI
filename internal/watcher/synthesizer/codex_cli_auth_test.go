package synthesizer

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
)

func TestConfigSynthesizer_CodexCLIAuth(t *testing.T) {
	tmpDir := t.TempDir()
	authFile := filepath.Join(tmpDir, "auth.json")
	configFile := filepath.Join(tmpDir, "config.toml")

	if err := os.WriteFile(authFile, []byte(`{"OPENAI_API_KEY":"sk-test-codex-cli"}`), 0o600); err != nil {
		t.Fatalf("write auth file: %v", err)
	}
	if err := os.WriteFile(configFile, []byte(`
model_provider = "baibai"

[model_providers.baibai]
name = "OpenAI"
base_url = "https://api.sharesai.xyz/v1"
wire_api = "responses"
requires_openai_auth = true
`), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	synth := NewConfigSynthesizer()
	ctx := &SynthesisContext{
		Config: &config.Config{
			CodexCLIAuth: []config.CodexCLIAuthSource{
				{
					Enable:     true,
					AuthFile:   authFile,
					ConfigFile: configFile,
					Prefix:     "codex-cli",
					Websockets: true,
					Headers:    map[string]string{"X-Test": "value"},
					ProxyURL:   "http://proxy.local",
				},
			},
		},
		Now:         time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC),
		IDGenerator: NewStableIDGenerator(),
	}

	auths, err := synth.Synthesize(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(auths) != 1 {
		t.Fatalf("expected 1 auth, got %d", len(auths))
	}

	auth := auths[0]
	if auth.Provider != "codex" {
		t.Fatalf("provider = %s, want codex", auth.Provider)
	}
	if auth.Label != "OpenAI" {
		t.Fatalf("label = %s, want OpenAI", auth.Label)
	}
	if auth.Prefix != "codex-cli" {
		t.Fatalf("prefix = %s, want codex-cli", auth.Prefix)
	}
	if auth.ProxyURL != "http://proxy.local" {
		t.Fatalf("proxy_url = %s, want http://proxy.local", auth.ProxyURL)
	}
	if got := auth.Attributes["api_key"]; got != "sk-test-codex-cli" {
		t.Fatalf("api_key = %s, want sk-test-codex-cli", got)
	}
	if got := auth.Attributes["base_url"]; got != "https://api.sharesai.xyz/v1" {
		t.Fatalf("base_url = %s, want https://api.sharesai.xyz/v1", got)
	}
	if got := auth.Attributes["codex_cli_provider"]; got != "baibai" {
		t.Fatalf("codex_cli_provider = %s, want baibai", got)
	}
	if got := auth.Attributes["codex_cli_api_key_field"]; got != "OPENAI_API_KEY" {
		t.Fatalf("codex_cli_api_key_field = %s, want OPENAI_API_KEY", got)
	}
	if got := auth.Attributes["websockets"]; got != "true" {
		t.Fatalf("websockets = %s, want true", got)
	}
	if got := auth.Attributes["header:X-Test"]; got != "value" {
		t.Fatalf("header:X-Test = %s, want value", got)
	}
	if got := auth.Attributes["auth_kind"]; got != "apikey" {
		t.Fatalf("auth_kind = %s, want apikey", got)
	}
}

func TestConfigSynthesizer_CodexCLIAuth_SkipsUnsupportedWireAPI(t *testing.T) {
	tmpDir := t.TempDir()
	authFile := filepath.Join(tmpDir, "auth.json")
	configFile := filepath.Join(tmpDir, "config.toml")

	if err := os.WriteFile(authFile, []byte(`{"OPENAI_API_KEY":"sk-test-codex-cli"}`), 0o600); err != nil {
		t.Fatalf("write auth file: %v", err)
	}
	if err := os.WriteFile(configFile, []byte(`
model_provider = "baibai"

[model_providers.baibai]
name = "OpenAI"
base_url = "https://api.sharesai.xyz/v1"
wire_api = "chat_completions"
requires_openai_auth = true
`), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	synth := NewConfigSynthesizer()
	ctx := &SynthesisContext{
		Config: &config.Config{
			CodexCLIAuth: []config.CodexCLIAuthSource{{
				Enable:     true,
				AuthFile:   authFile,
				ConfigFile: configFile,
			}},
		},
		Now:         time.Now(),
		IDGenerator: NewStableIDGenerator(),
	}

	auths, err := synth.Synthesize(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(auths) != 0 {
		t.Fatalf("expected 0 auths, got %d", len(auths))
	}
}

func TestConfigSynthesizer_CodexCLIAuth_CustomAPIKeyField(t *testing.T) {
	tmpDir := t.TempDir()
	authFile := filepath.Join(tmpDir, "auth.json")
	configFile := filepath.Join(tmpDir, "config.toml")

	if err := os.WriteFile(authFile, []byte(`{"CLI_PROXY_API_OPENAI_API_KEY":"sk-test-fallback"}`), 0o600); err != nil {
		t.Fatalf("write auth file: %v", err)
	}
	if err := os.WriteFile(configFile, []byte(`
model_provider = "openai"

[model_providers.shareai]
name = "ShareAI"
base_url = "https://api.sharesai.xyz/v1"
wire_api = "responses"
requires_openai_auth = true
`), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	synth := NewConfigSynthesizer()
	ctx := &SynthesisContext{
		Config: &config.Config{
			CodexCLIAuth: []config.CodexCLIAuthSource{{
				Enable:      true,
				AuthFile:    authFile,
				APIKeyField: "CLI_PROXY_API_OPENAI_API_KEY",
				ConfigFile:  configFile,
				Provider:    "shareai",
			}},
		},
		Now:         time.Now(),
		IDGenerator: NewStableIDGenerator(),
	}

	auths, err := synth.Synthesize(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(auths) != 1 {
		t.Fatalf("expected 1 auth, got %d", len(auths))
	}
	auth := auths[0]
	if got := auth.Attributes["api_key"]; got != "sk-test-fallback" {
		t.Fatalf("api_key = %s, want sk-test-fallback", got)
	}
	if got := auth.Attributes["base_url"]; got != "https://api.sharesai.xyz/v1" {
		t.Fatalf("base_url = %s, want https://api.sharesai.xyz/v1", got)
	}
	if got := auth.Attributes["codex_cli_provider"]; got != "shareai" {
		t.Fatalf("codex_cli_provider = %s, want shareai", got)
	}
	if got := auth.Attributes["codex_cli_api_key_field"]; got != "CLI_PROXY_API_OPENAI_API_KEY" {
		t.Fatalf("codex_cli_api_key_field = %s, want CLI_PROXY_API_OPENAI_API_KEY", got)
	}
}
