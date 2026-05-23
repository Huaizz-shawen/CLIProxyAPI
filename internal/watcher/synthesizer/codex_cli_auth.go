package synthesizer

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/util"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	log "github.com/sirupsen/logrus"
)

type codexCLIConfigTOML struct {
	ModelProvider  string                             `toml:"model_provider"`
	ModelProviders map[string]codexCLIConfigTOMLEntry `toml:"model_providers"`
}

type codexCLIConfigTOMLEntry struct {
	Name               string `toml:"name"`
	BaseURL            string `toml:"base_url"`
	WireAPI            string `toml:"wire_api"`
	RequiresOpenAIAuth bool   `toml:"requires_openai_auth"`
}

type resolvedCodexCLIAuthSource struct {
	APIKey       string
	BaseURL      string
	ProviderKey  string
	ProviderName string
	AuthFile     string
	APIKeyField  string
	ConfigFile   string
}

// synthesizeCodexCLIAuth creates runtime Auth entries from Codex CLI auth.json
// plus config.toml sources.
func (s *ConfigSynthesizer) synthesizeCodexCLIAuth(ctx *SynthesisContext) []*coreauth.Auth {
	cfg := ctx.Config
	now := ctx.Now
	idGen := ctx.IDGenerator

	out := make([]*coreauth.Auth, 0, len(cfg.CodexCLIAuth))
	for i := range cfg.CodexCLIAuth {
		entry := cfg.CodexCLIAuth[i]
		if !entry.Enable {
			continue
		}

		resolved, err := resolveCodexCLIAuthSource(&entry)
		if err != nil {
			log.Warnf("codex-cli-auth[%d]: %v", i, err)
			continue
		}

		id, token := idGen.Next(
			"codex:codex-cli",
			resolved.APIKey,
			resolved.BaseURL,
			resolved.ProviderKey,
			resolved.AuthFile,
			resolved.ConfigFile,
			strings.TrimSpace(entry.ProxyURL),
		)
		attrs := map[string]string{
			"source":                   fmt.Sprintf("config:codex-cli[%s]", token),
			"api_key":                  resolved.APIKey,
			"base_url":                 resolved.BaseURL,
			"codex_cli_auth_file":      resolved.AuthFile,
			"codex_cli_api_key_field":  resolved.APIKeyField,
			"codex_cli_config_file":    resolved.ConfigFile,
			"codex_cli_provider":       resolved.ProviderKey,
			"codex_cli_provider_label": resolved.ProviderName,
			"codex_cli_requires_auth":  strconv.FormatBool(true),
			"codex_cli_source_enabled": strconv.FormatBool(true),
		}
		if entry.Priority != 0 {
			attrs["priority"] = strconv.Itoa(entry.Priority)
		}
		if entry.Websockets {
			attrs["websockets"] = "true"
		}
		addConfigHeadersToAttrs(entry.Headers, attrs)

		label := "codex-cli"
		if strings.TrimSpace(resolved.ProviderName) != "" {
			label = strings.TrimSpace(resolved.ProviderName)
		}

		auth := &coreauth.Auth{
			ID:         id,
			Provider:   "codex",
			Label:      label,
			Prefix:     strings.TrimSpace(entry.Prefix),
			Status:     coreauth.StatusActive,
			ProxyURL:   strings.TrimSpace(entry.ProxyURL),
			Attributes: attrs,
			CreatedAt:  now,
			UpdatedAt:  now,
			Metadata: map[string]any{
				"source":                  "codex-cli",
				"codex_cli_provider":      resolved.ProviderKey,
				"codex_cli_provider_name": resolved.ProviderName,
				"codex_cli_auth_file":     resolved.AuthFile,
				"codex_cli_config_file":   resolved.ConfigFile,
			},
		}
		ApplyAuthExcludedModelsMeta(auth, cfg, entry.ExcludedModels, "apikey")
		out = append(out, auth)
	}
	return out
}

func resolveCodexCLIAuthSource(entry *config.CodexCLIAuthSource) (*resolvedCodexCLIAuthSource, error) {
	if entry == nil {
		return nil, fmt.Errorf("entry is nil")
	}

	authFile, err := util.ResolveAuthDir(entry.AuthFile)
	if err != nil {
		return nil, fmt.Errorf("resolve auth-file: %w", err)
	}
	configFile, err := util.ResolveAuthDir(entry.ConfigFile)
	if err != nil {
		return nil, fmt.Errorf("resolve config-file: %w", err)
	}

	authData, err := os.ReadFile(authFile)
	if err != nil {
		return nil, fmt.Errorf("read auth-file %s: %w", authFile, err)
	}
	var authJSON map[string]any
	if err = json.Unmarshal(authData, &authJSON); err != nil {
		return nil, fmt.Errorf("parse auth-file %s: %w", authFile, err)
	}
	apiKeyField := strings.TrimSpace(entry.APIKeyField)
	if apiKeyField == "" {
		apiKeyField = "OPENAI_API_KEY"
	}
	apiKey := strings.TrimSpace(stringFromJSONMap(authJSON, apiKeyField))
	if apiKey == "" {
		return nil, fmt.Errorf("auth-file %s does not contain %s", authFile, apiKeyField)
	}

	configData, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("read config-file %s: %w", configFile, err)
	}
	var tomlConfig codexCLIConfigTOML
	if err = toml.Unmarshal(configData, &tomlConfig); err != nil {
		return nil, fmt.Errorf("parse config-file %s: %w", configFile, err)
	}

	providerKey := strings.TrimSpace(entry.Provider)
	if providerKey == "" {
		providerKey = strings.TrimSpace(tomlConfig.ModelProvider)
	}
	if providerKey == "" && len(tomlConfig.ModelProviders) == 1 {
		for key := range tomlConfig.ModelProviders {
			providerKey = strings.TrimSpace(key)
		}
	}
	if providerKey == "" {
		return nil, fmt.Errorf("config-file %s does not specify model_provider", configFile)
	}

	provider, ok := tomlConfig.ModelProviders[providerKey]
	if !ok {
		return nil, fmt.Errorf("config-file %s missing model_providers.%s", configFile, providerKey)
	}

	baseURL := strings.TrimSpace(provider.BaseURL)
	if baseURL == "" {
		return nil, fmt.Errorf("config-file %s provider %s has empty base_url", configFile, providerKey)
	}
	wireAPI := strings.TrimSpace(provider.WireAPI)
	if wireAPI != "" && !strings.EqualFold(wireAPI, "responses") {
		return nil, fmt.Errorf("config-file %s provider %s uses unsupported wire_api %q", configFile, providerKey, wireAPI)
	}
	if !provider.RequiresOpenAIAuth {
		return nil, fmt.Errorf("config-file %s provider %s does not require_openai_auth", configFile, providerKey)
	}

	return &resolvedCodexCLIAuthSource{
		APIKey:       apiKey,
		BaseURL:      baseURL,
		ProviderKey:  providerKey,
		ProviderName: strings.TrimSpace(provider.Name),
		AuthFile:     authFile,
		APIKeyField:  apiKeyField,
		ConfigFile:   configFile,
	}, nil
}

func stringFromJSONMap(values map[string]any, key string) string {
	if len(values) == 0 || key == "" {
		return ""
	}
	raw, ok := values[key]
	if !ok || raw == nil {
		return ""
	}
	switch v := raw.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return ""
	}
}
