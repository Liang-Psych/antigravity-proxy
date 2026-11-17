package proxy

import (
	"encoding/json"
	"math"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// upstreamDiagnostic contains only fixed categories, instructions and validated
// schema paths. Provider messages, metadata and resource identities never escape.
type upstreamDiagnostic struct {
	Origin            string   `json:"origin"`
	Category          string   `json:"category"`
	Action            string   `json:"action"`
	Fields            []string `json:"fields,omitempty"`
	RetryDelaySeconds int64    `json:"retry_delay_seconds,omitempty"`
}

var schemaFieldPath = regexp.MustCompile(`^(contents|parts|text|role|inlineData|mimeType|data|functionCall|functionResponse|name|args|response|systemInstruction|generationConfig|temperature|topP|topK|maxOutputTokens|thinkingConfig|thinkingBudget|thinkingLevel|includeThoughts|responseMimeType|responseSchema|responseJsonSchema|tools|functionDeclarations|parameters|parametersJsonSchema|toolConfig|functionCallingConfig|mode|allowedFunctionNames|safetySettings|category|threshold|candidateCount|stopSequences)(\[[0-9]{1,6}\])?$`)

func safeSchemaPath(field string) bool {
	if len(field) == 0 || len(field) > 160 {
		return false
	}
	parts := strings.Split(field, ".")
	switch strings.Split(parts[0], "[")[0] {
	case "contents", "systemInstruction", "generationConfig", "tools", "toolConfig", "safetySettings":
	default:
		return false
	}
	for _, part := range parts {
		if !schemaFieldPath.MatchString(part) {
			return false
		}
	}
	return true
}

func diagnoseUpstream(status int, body []byte) upstreamDiagnostic {
	d := upstreamDiagnostic{Origin: "upstream", Category: "upstream_error", Action: "Inspect the proxy request ID and upstream HTTP status."}
	switch status {
	case 400:
		d.Category, d.Action = "invalid_request", "Check the native Gemini request fields and model capabilities."
	case 401:
		d.Category, d.Action = "authentication_failed", "Reauthenticate the account or replace an explicit access token."
	case 403:
		d.Category, d.Action = "permission_denied", "Check account access and project permissions."
	case 404:
		d.Category, d.Action = "not_found", "Refresh the account model catalog and check the exact model ID."
	case 429:
		d.Category, d.Action = "rate_limited", "Wait for Retry-After or check the account quota."
	}
	var envelope struct {
		Error struct {
			Message string `json:"message"`
			Details []struct {
				Type            string `json:"@type"`
				Reason          string `json:"reason"`
				Domain          string `json:"domain"`
				RetryDelay      string `json:"retryDelay"`
				FieldViolations []struct {
					Field string `json:"field"`
				} `json:"fieldViolations"`
			} `json:"details"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		return d
	}
	// A narrow diagnostic hint, never an authorization or replay decision.
	message := strings.ToLower(envelope.Error.Message)
	if (status == 400 || status == 403) && strings.Contains(message, "version") && (strings.Contains(message, "no longer supported") || strings.Contains(message, "outdated") || strings.Contains(message, "unsupported client version")) {
		d.Category, d.Action = "client_version_rejected", "Update the configured client version and verify its client family."
	}
	seen := map[string]bool{}
	for _, detail := range envelope.Error.Details {
		switch detail.Type {
		case "type.googleapis.com/google.rpc.BadRequest":
			if status != http.StatusBadRequest {
				continue
			}
			for _, violation := range detail.FieldViolations {
				if len(d.Fields) < 16 && safeSchemaPath(violation.Field) && !seen[violation.Field] {
					d.Fields = append(d.Fields, violation.Field)
					seen[violation.Field] = true
				}
			}
		case "type.googleapis.com/google.rpc.RetryInfo":
			if status != 429 && status != 503 {
				continue
			}
			duration, err := time.ParseDuration(detail.RetryDelay)
			if err == nil && strings.HasSuffix(detail.RetryDelay, "s") && duration > 0 && duration <= 7*24*time.Hour {
				delay := int64(math.Ceil(duration.Seconds()))
				if delay > d.RetryDelaySeconds {
					d.RetryDelaySeconds = delay
				}
			}
		case "type.googleapis.com/google.rpc.ErrorInfo":
			if detail.Domain != "googleapis.com" && detail.Domain != "cloudcode-pa.googleapis.com" {
				continue
			}
			if (status == 400 || status == 403) && (detail.Reason == "CLIENT_VERSION_UNSUPPORTED" || detail.Reason == "CLIENT_VERSION_OUTDATED") {
				d.Category, d.Action = "client_version_rejected", "Update the configured client version and verify its client family."
			}
		}
	}
	return d
}
