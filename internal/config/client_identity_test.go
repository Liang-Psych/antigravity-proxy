package config

import (
	"os"
	"strings"
	"testing"
)

func TestClientIdentityEnvironmentAndPersistence(t *testing.T) {
	home := isolatedLoadHome(t)
	path := writeLoadConfig(t, home, `{"cliClientVersion":"1.4.0","discoveryClientFamily":"standard","generationClientRules":[]}`)
	cfg, _, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CLIClientVersion != "1.4.0" || cfg.DiscoveryClientFamily != "standard" || cfg.GenerationClientFamily("gemini-4-test") != "standard" {
		t.Fatal("disk settings lost")
	}
	// Account administration must preserve an explicit empty rule list.
	if err := UpdateAccounts(path, func(accounts *[]Account) error { return nil }); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), `"generationClientRules": []`) {
		t.Fatalf("empty rules lost: %v", err)
	}
	t.Setenv("ANTIGRAVITY_CLI_CLIENT_VERSION", "1.5.0")
	t.Setenv("ANTIGRAVITY_DISCOVERY_CLIENT_FAMILY", "cli")
	t.Setenv("ANTIGRAVITY_GENERATION_CLIENT_RULES", `[{"modelPrefix":"new-","family":"cli"},{"modelPrefix":"","family":"standard"}]`)
	cfg, _, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CLIClientVersion != "1.5.0" || cfg.DiscoveryClientFamily != "cli" || cfg.GenerationClientFamily("new-model") != "cli" || cfg.GenerationClientFamily("gemini-4-test") != "standard" {
		t.Fatal("environment overrides lost")
	}
}

func TestClientIdentityRejectsInvalidSettings(t *testing.T) {
	for _, setting := range []string{
		`{"cliClientVersion":"bad\r\nHeader:value"}`, `{"discoveryClientFamily":"typo"}`,
		`{"generationClientRules":[{"modelPrefix":"x","family":"typo"}]}`,
		`{"generationClientRules":[{"modelPrefix":"bad prefix","family":"cli"}]}`,
	} {
		t.Run(setting, func(t *testing.T) {
			home := isolatedLoadHome(t)
			writeLoadConfig(t, home, setting)
			if _, _, err := Load(); err == nil {
				t.Fatal("invalid identity accepted")
			}
		})
	}
	for _, rules := range []string{`null`, `{}`, `broken`, `[] []`, `[{"prefix":"x","family":"cli"}]`} {
		t.Run(rules, func(t *testing.T) {
			isolatedLoadHome(t)
			t.Setenv("ANTIGRAVITY_GENERATION_CLIENT_RULES", rules)
			if _, _, err := Load(); err == nil {
				t.Fatal("invalid rules accepted")
			}
		})
	}
}
