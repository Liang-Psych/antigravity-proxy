package proxy

import (
	"antigravity-proxy/internal/config"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// TestLiveNativeCompatibility is opt-in: it consumes the selected account's
// quota. It never saves credentials, modifies configuration or logs model text.
func TestLiveNativeCompatibility(t *testing.T) {
	model := os.Getenv("AGY_LIVE_MODEL")
	if model == "" {
		t.Skip("set AGY_LIVE_MODEL to opt into real upstream requests")
	}
	if strings.ContainsAny(model, "/:?#") {
		t.Fatal("use an exact model ID without a path or query")
	}
	cfg, _, err := config.Load()
	if err != nil {
		t.Fatal("cannot load private proxy configuration")
	}
	selected := os.Getenv("AGY_LIVE_ACCOUNT")
	found := false
	for _, account := range cfg.Accounts {
		if !account.Enabled || !account.HasCredentials() || (selected != "" && account.ID != selected) {
			continue
		}
		if found {
			t.Fatal("multiple usable accounts; set AGY_LIVE_ACCOUNT")
		}
		cfg.AccessToken, cfg.RefreshToken = account.AccessToken, account.RefreshToken
		cfg.OAuthClientID, cfg.OAuthClientSecret = account.OAuthClientID, account.OAuthClientSecret
		if account.ProjectID != "" {
			cfg.ProjectID = account.ProjectID
		}
		found = true
	}
	if !found {
		t.Fatal("no matching enabled account with credentials")
	}
	cfg.APIKey = ""
	p := New(cfg)
	defer p.client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	// Failures report status, never provider messages or generated payloads.
	invoke := func(path, body string) *httptest.ResponseRecorder {
		t.Helper()
		method := http.MethodGet
		if body != "" {
			method = http.MethodPost
		}
		response := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body)).WithContext(ctx)
		p.ServeHTTP(response, req)
		if response.Code != http.StatusOK {
			t.Fatalf("live operation failed with HTTP %d (body omitted)", response.Code)
		}
		return response
	}
	catalog := invoke("/v1beta/models", "")
	var list geminiModelList
	if err := json.Unmarshal(catalog.Body.Bytes(), &list); err != nil {
		t.Fatal("invalid native catalog")
	}
	// Pagination-independent exact lookup proves visibility before generation.
	invoke("/v1beta/models/"+model, "")
	t.Log("catalog_visible=true; generation not yet verified")
	body := `{"contents":[{"role":"user","parts":[{"text":"Reply with exactly OK."}]}],"generationConfig":{"maxOutputTokens":64}}`
	response := invoke("/v1beta/models/"+model+":generateContent", body)
	var result struct {
		Candidates []json.RawMessage `json:"candidates"`
	}
	if json.Unmarshal(response.Body.Bytes(), &result) != nil || len(result.Candidates) == 0 {
		t.Fatal("generation returned no candidates")
	}
	t.Log("generation_succeeded=true")
	// Calling the stream forwarder through an HTTP server exercises real flush and
	// abort behavior; a recorder alone cannot prove a stream completed correctly.
	server := httptest.NewServer(p)
	defer server.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/v1beta/models/"+model+":streamGenerateContent?alt=sse", strings.NewReader(body))
	if err != nil {
		t.Fatal("create stream request")
	}
	req.Header.Set("Content-Type", "application/json")
	streamed, err := server.Client().Do(req)
	if err != nil {
		t.Fatal("live stream failed before response")
	}
	defer streamed.Body.Close()
	data, err := io.ReadAll(io.LimitReader(streamed.Body, 1<<20))
	if err != nil || streamed.StatusCode != 200 || !strings.Contains(string(data), `"finishReason"`) {
		t.Fatal("live stream did not complete")
	}
	t.Log("stream_succeeded=true")
}
