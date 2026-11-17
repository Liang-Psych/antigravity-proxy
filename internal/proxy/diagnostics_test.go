package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestUpstreamDiagnosticsAreActionableAndSanitized(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		status                int
		body, category, retry string
		fields                int
	}{
		{"bad field", 400, `{"error":{"message":"private prompt and token","details":[{"@type":"type.googleapis.com/google.rpc.BadRequest","fieldViolations":[{"field":"contents[0].parts[1].inlineData.mimeType","description":"secret"},{"field":"generationConfig.thinkingConfig.thinkingBudget"},{"field":"contents.secret-token"},{"field":"refreshToken"},{"field":"contents[0].parts[1].inlineData.mimeType"}]}]}}`, "invalid_request", "", 2},
		{"quota", 429, `{"error":{"message":"private account","details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"1.2s"}]}}`, "rate_limited", "2", 0},
		{"version", 403, `{"error":{"message":"private","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","domain":"googleapis.com","reason":"CLIENT_VERSION_UNSUPPORTED","metadata":{"token":"secret"}}]}}`, "client_version_rejected", "", 0},
		{"version hint", 400, `{"error":{"message":"This version is no longer supported; private@example.com"}}`, "client_version_rejected", "", 0},
		{"untrusted reason", 403, `{"error":{"details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","domain":"private.example.com","reason":"CLIENT_VERSION_UNSUPPORTED"}]}}`, "permission_denied", "", 0},
		{"invalid JSON", 429, `private secret-token`, "rate_limited", "", 0},
		{"oversize", 429, strings.Repeat("x", 17<<10), "rate_limited", "", 0},
		{"invalid delay", 429, `{"error":{"details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"999999999999s"}]}}`, "rate_limited", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer upstream.Close()
			p := generationTestProxy(upstream.URL)
			w := httptest.NewRecorder()
			p.ServeHTTP(w, httptest.NewRequest("POST", "/v1beta/models/test:generateContent", strings.NewReader(`{"contents":[]}`)))
			var result struct {
				Error struct {
					Diagnostic upstreamDiagnostic `json:"diagnostic"`
				} `json:"error"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			d := result.Error.Diagnostic
			if w.Code != tc.status || d.Origin != "upstream" || d.Category != tc.category || len(d.Fields) != tc.fields || w.Header().Get("Retry-After") != tc.retry {
				t.Fatalf("unexpected diagnostic: %d %+v %v", w.Code, d, w.Header())
			}
			for _, secret := range []string{"private", "secret", "@example.com"} {
				if strings.Contains(w.Body.String(), secret) {
					t.Fatal("provider content leaked")
				}
			}
		})
	}
}

func TestDiagnosticErrorBodyTimeoutPreservesStatus(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(429)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer upstream.Close()
	p := generationTestProxy(upstream.URL)
	w := httptest.NewRecorder()
	started := time.Now()
	p.ServeHTTP(w, httptest.NewRequest("POST", "/v1beta/models/test:generateContent", strings.NewReader(`{}`)))
	if w.Code != 429 || w.Header().Get("Retry-After") != "7" || time.Since(started) > 2*time.Second {
		t.Fatalf("error read lost original status or hung: %d", w.Code)
	}
}

func TestLocalCapacityDiagnosticDiffersFromUpstream(t *testing.T) {
	p := generationTestProxy("http://127.0.0.1:1")
	p.generationQueueWait = 0
	for i := 0; i < cap(p.generationSlots); i++ {
		p.generationSlots <- struct{}{}
	}
	w := httptest.NewRecorder()
	p.ServeHTTP(w, httptest.NewRequest("POST", "/v1beta/models/test:generateContent", strings.NewReader(`{}`)))
	if w.Code != 429 || !strings.Contains(w.Body.String(), `"origin":"proxy"`) || !strings.Contains(w.Body.String(), "generation_capacity_exhausted") {
		t.Fatalf("ambiguous local limit: %d %s", w.Code, w.Body.String())
	}
}
