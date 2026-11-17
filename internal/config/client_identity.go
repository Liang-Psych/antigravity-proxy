package config

import (
	"fmt"
	"strings"
)

// Client families change transport identity only, never model IDs or payloads.
const (
	ClientFamilyStandard    = "standard"
	ClientFamilyCLI         = "cli"
	DefaultCLIClientVersion = "1.5.10"
)

// ClientFamilyRule selects the first matching model prefix. An empty prefix matches all models.
type ClientFamilyRule struct {
	ModelPrefix string `json:"modelPrefix"`
	Family      string `json:"family"`
}

func defaultClientRules() []ClientFamilyRule {
	return []ClientFamilyRule{{ModelPrefix: "gemini-4-", Family: ClientFamilyCLI}}
}

// GenerationClientFamily falls back to the standard identity for unmatched models.
func (cfg Config) GenerationClientFamily(model string) string {
	rules := cfg.GenerationClientRules
	if rules == nil {
		rules = defaultClientRules()
	}
	for _, rule := range rules {
		if strings.HasPrefix(model, rule.ModelPrefix) {
			return rule.Family
		}
	}
	return ClientFamilyStandard
}

func (cfg Config) validateClientIdentity() error {
	validVersion := func(version string) bool {
		if len(version) == 0 || len(version) > 64 {
			return false
		}
		for _, c := range version {
			if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._-", c) {
				return false
			}
		}
		return true
	}
	if !validVersion(cfg.ClientVersion) {
		return fmt.Errorf("clientVersion must contain 1-64 letters, digits, dots, underscores or hyphens")
	}
	if cfg.CLIClientVersion != "" && !validVersion(cfg.CLIClientVersion) {
		return fmt.Errorf("cliClientVersion must contain 1-64 letters, digits, dots, underscores or hyphens")
	}
	validFamily := func(family string) bool { return family == ClientFamilyStandard || family == ClientFamilyCLI }
	if cfg.DiscoveryClientFamily != "" && !validFamily(cfg.DiscoveryClientFamily) {
		return fmt.Errorf("discoveryClientFamily must be standard or cli")
	}
	for _, rule := range cfg.GenerationClientRules {
		if !validFamily(rule.Family) {
			return fmt.Errorf("generationClientRules family must be standard or cli")
		}
		if rule.ModelPrefix != "" && !validVersion(rule.ModelPrefix) {
			return fmt.Errorf("generationClientRules modelPrefix must be an empty catch-all or a model prefix of at most 64 letters, digits, dots, underscores or hyphens")
		}
	}
	return nil
}
