package proxy

import (
	"antigravity-proxy/internal/config"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

func expectedCLIUserAgent() string {
	return fmt.Sprintf("antigravity/cli/1.3.4 (os_type=%s; arch=%s; aidev_client; auth_method=oauth)", runtime.GOOS, runtime.GOARCH)
}

func TestModelDiscoveryUsesCLIClientFamily(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/v1internal:fetchAvailableModels" {
			t.Errorf("discovery route = %s %s", r.Method, r.URL)
		}
		if got := r.Header.Get("User-Agent"); got != expectedCLIUserAgent() {
			t.Errorf("discovery User-Agent = %q, want CLI family", got)
		}
		if r.Header.Get("X-Client-Version") != "custom-version" || r.Header.Get("X-Client-Name") != "antigravity" || r.Header.Get("Authorization") != "Bearer discovery-token" {
			t.Error("discovery changed non-UA identity headers")
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"project":"project"}` {
			t.Errorf("discovery body = %s", body)
		}
		_, _ = io.WriteString(w, `{"models":{"gemini-4-argon-medium":{"apiProvider":"API_PROVIDER_GOOGLE"}}}`)
	}))
	defer upstream.Close()
	p := New(config.Config{AccessToken: "discovery-token", ProjectID: "project", ClientVersion: "custom-version", DailyEndpoint: upstream.URL, ProdEndpoint: upstream.URL})
	for _, path := range []string{"/v1beta/models", "/models"} {
		response := httptest.NewRecorder()
		p.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "gemini-4-argon-medium") {
			t.Fatalf("%s response = %d %s", path, response.Code, response.Body.String())
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("discovery requests = %d, want 2", calls.Load())
	}
}

func TestGenerationSelectsClientFamilyWithoutRewritingPayload(t *testing.T) {
	const request = `{"contents":[{"role":"model","parts":[{"thoughtSignature":"opaque+/=="},{"functionCall":{"name":"tool","args":{"n":9007199254740993123}}}]}],"future":1.000e+40}`
	for _, model := range []string{"gemini-4-argon-medium", "gemini-4-hydrogen-xhigh", "gemini-3.8-flash-medium", "claude-sonnet-4-6", "gemini-40-test"} {
		for _, action := range []string{"generateContent", "streamGenerateContent"} {
			t.Run(model+"/"+action, func(t *testing.T) {
				var calls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					wantUA := fmt.Sprintf("antigravity/custom-version %s/%s", runtime.GOOS, runtime.GOARCH)
					if strings.HasPrefix(model, "gemini-4-") {
						wantUA = expectedCLIUserAgent()
					}
					if got := r.Header.Get("User-Agent"); got != wantUA {
						t.Errorf("User-Agent = %q, want %q", got, wantUA)
					}
					if r.Header.Get("X-Client-Version") != "custom-version" || r.Header.Get("Authorization") != "Bearer token" {
						t.Error("generation changed non-UA identity headers")
					}
					if r.URL.Path != "/v1internal:"+action || (action == "streamGenerateContent" && r.URL.Query().Get("alt") != "sse") {
						t.Errorf("generation route = %s", r.URL)
					}
					var envelope struct {
						Model       string          `json:"model"`
						Project     string          `json:"project"`
						UserAgent   string          `json:"userAgent"`
						RequestType string          `json:"requestType"`
						Request     json.RawMessage `json:"request"`
					}
					if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
						t.Error(err)
					}
					if envelope.Model != model || envelope.Project != "project" || envelope.UserAgent != "antigravity" || envelope.RequestType != "agent" || string(envelope.Request) != request {
						t.Errorf("generation changed envelope or raw request: %+v", envelope)
					}
					if action == "streamGenerateContent" {
						_, _ = io.WriteString(w, "data: {\"response\":{\"candidates\":[{\"finishReason\":\"STOP\"}]}}\n\n")
					} else {
						_, _ = io.WriteString(w, `{"response":{"candidates":[{"finishReason":"STOP"}]}}`)
					}
				}))
				defer upstream.Close()
				p := New(config.Config{AccessToken: "token", ProjectID: "project", ClientVersion: "custom-version", DailyEndpoint: upstream.URL, ProdEndpoint: upstream.URL})
				response := httptest.NewRecorder()
				path := "/v1beta/models/" + model + ":" + action
				if action == "streamGenerateContent" {
					path += "?alt=sse"
				}
				p.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, strings.NewReader(request)))
				if response.Code != http.StatusOK || calls.Load() != 1 {
					t.Fatalf("generation response = %d %s, sends = %d", response.Code, response.Body.String(), calls.Load())
				}
			})
		}
	}
}

func TestClientFamilySurvivesOAuthRecovery(t *testing.T) {
	for _, mode := range []string{"discovery", "gemini-4-argon-medium", "gemini-3.8-flash-medium"} {
		t.Run(mode, func(t *testing.T) {
			var calls, refreshes atomic.Int32
			var firstBody string
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := calls.Add(1)
				wantUA := expectedCLIUserAgent()
				if mode == "gemini-3.8-flash-medium" {
					wantUA = fmt.Sprintf("antigravity/custom-version %s/%s", runtime.GOOS, runtime.GOARCH)
				}
				if got := r.Header.Get("User-Agent"); got != wantUA {
					t.Errorf("attempt %d User-Agent = %q, want %q", n, got, wantUA)
				}
				body, _ := io.ReadAll(r.Body)
				if n == 1 {
					firstBody = string(body)
					if r.Header.Get("Authorization") != "Bearer old-token" {
						t.Error("first request did not use old token")
					}
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				if string(body) != firstBody || r.Header.Get("Authorization") != "Bearer new-token" {
					t.Error("OAuth recovery changed request bytes or failed to replace the token")
				}
				_, _ = io.WriteString(w, `{"response":{}}`)
			}))
			defer upstream.Close()
			p := recoveryProxy(t, upstream.URL, func(w http.ResponseWriter, _ *http.Request) {
				refreshes.Add(1)
				_, _ = io.WriteString(w, `{"access_token":"new-token","expires_in":3600}`)
			})
			p.cfg.ClientVersion = "custom-version"
			var response *http.Response
			var err error
			if mode == "discovery" {
				response, err = p.postJSON(context.Background(), "old-token", "/v1internal:fetchAvailableModels", "project", map[string]string{"project": "project"})
			} else {
				payload := generationEnvelope{Project: "project", Model: mode, UserAgent: "antigravity", RequestType: "agent", Request: json.RawMessage(`{"future":9007199254740993123}`)}
				body, length, bodyErr := payload.requestBody()
				if bodyErr != nil {
					t.Fatal(bodyErr)
				}
				response, err = p.post(context.Background(), "old-token", upstreamCall{path: "/v1internal:generateContent", project: "project", accept: "application/json", body: body, length: length, cli: strings.HasPrefix(payload.Model, "gemini-4-")})
			}
			if err != nil {
				t.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			if calls.Load() != 2 || refreshes.Load() != 1 {
				t.Fatalf("requests=%d refreshes=%d", calls.Load(), refreshes.Load())
			}
		})
	}
}

func TestModelCatalogFiltersDisabledMetadataWithoutNameHeuristics(t *testing.T) {
	const catalog = `{"models":{
		"gemini-4-carbon-disabled":{"apiProvider":"API_PROVIDER_GOOGLE","disabled":true},
		"gemini-4-disabled-in-name":{"apiProvider":"API_PROVIDER_GOOGLE","disabled":false},
		"gemini-4-argon-low":{"apiProvider":"API_PROVIDER_GOOGLE"}
	}}`
	p := newCatalogTestProxy(t, func() string { return catalog })
	response := catalogTestRequest(p, "/v1beta/models")
	if response.Code != http.StatusOK {
		t.Fatalf("native catalog status = %d", response.Code)
	}
	var list geminiModelList
	if err := json.Unmarshal(response.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Models) != 2 || list.Models[0].Name != "models/gemini-4-argon-low" || list.Models[1].Name != "models/gemini-4-disabled-in-name" {
		t.Fatalf("metadata-filtered catalog = %+v", list.Models)
	}
	response = catalogTestRequest(p, "/models")
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != catalog {
		t.Fatalf("raw discovery catalog was filtered or rewritten: %d %s", response.Code, response.Body.String())
	}
}
