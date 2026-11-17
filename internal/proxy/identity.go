package proxy

import (
	"fmt"
	"runtime"
	"time"
)

const (
	defaultRPCTimeout         = 5 * time.Minute
	defaultHeaderTimeout      = 5 * time.Minute
	defaultBodyIdleTimeout    = 5 * time.Minute
	defaultOnboardingAttempts = 10
	defaultOnboardingInterval = 5 * time.Second
)

type clientInfo struct {
	IDEType    int `json:"ideType"`
	Platform   int `json:"platform"`
	PluginType int `json:"pluginType"`
}

func clientMetadata() clientInfo {
	return clientInfo{IDEType: 9, Platform: platformNumber(), PluginType: 2}
}

// The numeric identifiers belong to the provider protocol, not a local enum.
func platformNumber() int {
	switch runtime.GOOS {
	case "darwin":
		if runtime.GOARCH == "arm64" {
			return 2
		}
		return 1
	case "linux":
		if runtime.GOARCH == "arm64" {
			return 4
		}
		return 3
	case "windows":
		return 5
	default:
		return 0
	}
}

// The CLI family exposes Gemini 4 in discovery and generation. Its version is
// independent of ClientVersion, which still controls the other client headers.
func antigravityCLIUserAgent(version string) string {
	return fmt.Sprintf("antigravity/cli/%s (os_type=%s; arch=%s; aidev_client; auth_method=oauth)", version, runtime.GOOS, runtime.GOARCH)
}
