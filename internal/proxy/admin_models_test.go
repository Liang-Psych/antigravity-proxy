package proxy

import (
	"antigravity-proxy/internal/apikey"
	"antigravity-proxy/internal/config"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func newAdminModelsTestRuntime(t *testing.T) (*Runtime, string, string, string, *atomic.Int32, *atomic.Int32) {
	t.Helper()
	const adminKey, workKey, personalKey = "admin-models-fixture", "work-models-fixture", "personal-models-fixture"
	var workCalls, personalCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost || req.URL.Path != "/v1internal:fetchAvailableModels" {
			t.Errorf("unexpected upstream request: %s %s", req.Method, req.URL.Path)
			http.Error(w, "unexpected route", http.StatusNotFound)
			return
		}
		var account string
		switch req.Header.Get("Authorization") {
		case "Bearer token-work":
			account = "work"
			workCalls.Add(1)
		case "Bearer token-personal":
			account = "personal"
			personalCalls.Add(1)
		default:
			t.Error("upstream request used an unexpected credential")
			http.Error(w, "wrong account", http.StatusUnauthorized)
			return
		}
		var body struct {
			Project string `json:"project"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.Project != "project-"+account {
			t.Errorf("project for %s = %q, decode error %v", account, body.Project, err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"models": map[string]any{
			"gemini-" + account + "-a": map[string]string{"apiProvider": "API_PROVIDER_GOOGLE"},
			"gemini-" + account + "-b": map[string]string{"apiProvider": "API_PROVIDER_GOOGLE"},
		}})
	}))
	t.Cleanup(upstream.Close)
	accounts := make(map[string]*accountRuntime)
	for _, id := range []string{"work", "personal"} {
		p := New(config.Config{AccessToken: "token-" + id, ProjectID: "project-" + id, DailyEndpoint: upstream.URL, ProdEndpoint: upstream.URL})
		t.Cleanup(p.client.CloseIdleConnections)
		accounts[id] = &accountRuntime{id: id, enabled: true, proxy: p}
	}
	accounts["disabled"] = &accountRuntime{id: "disabled", enabled: false, proxy: accounts["work"].proxy}
	accounts["unavailable"] = &accountRuntime{id: "unavailable", enabled: true}
	keys, err := apikey.NewRegistry(map[string]apikey.Binding{
		apikey.Hash(workKey):     {AccountID: "work", KeyID: "work-key"},
		apikey.Hash(personalKey): {AccountID: "personal", KeyID: "personal-key"},
	})
	if err != nil {
		t.Fatal(err)
	}
	r := &Runtime{ctx: context.Background(), adminKeyHash: apikey.Hash(adminKey)}
	r.registry.Store(&runtimeRegistry{accounts: accounts, keys: keys})
	r.adminHandler = r.setupAdminMux()
	return r, adminKey, workKey, personalKey, &workCalls, &personalCalls
}

func adminModelsTestRequest(r *Runtime, path, key string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func decodeAdminModelsTestList(t *testing.T, response *httptest.ResponseRecorder) geminiModelList {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("models status = %d, body = %s", response.Code, response.Body.String())
	}
	var result geminiModelList
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestAdminAccountModelsUseSelectedAccountAndIsolatedCache(t *testing.T) {
	r, admin, workKey, personalKey, workCalls, personalCalls := newAdminModelsTestRuntime(t)
	read := func(path, key string) []string {
		response := adminModelsTestRequest(r, path, key)
		if strings.HasPrefix(path, "/admin/") && response.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("admin models response must not be publicly cached")
		}
		list := decodeAdminModelsTestList(t, response)
		names := make([]string, 0, len(list.Models))
		for _, model := range list.Models {
			names = append(names, model.Name)
		}
		return names
	}
	for i := 0; i < 2; i++ {
		for _, id := range []string{"work", "personal"} {
			got := read("/admin/accounts/"+id+"/models", admin)
			want := []string{"models/gemini-" + id + "-a", "models/gemini-" + id + "-b"}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("%s models = %v, want %v", id, got, want)
			}
		}
	}
	// The existing data route must remain bound to its own API key and the same account cache.
	if got := read("/v1beta/models", workKey); got[0] != "models/gemini-work-a" {
		t.Fatalf("work data route = %v", got)
	}
	if got := read("/v1beta/models", personalKey); got[0] != "models/gemini-personal-a" {
		t.Fatalf("personal data route = %v", got)
	}
	if workCalls.Load() != 1 || personalCalls.Load() != 1 {
		t.Fatalf("catalog calls: work=%d personal=%d, want 1 each", workCalls.Load(), personalCalls.Load())
	}
}

func TestAdminAccountModelsPreservePagination(t *testing.T) {
	r, admin, _, _, _, _ := newAdminModelsTestRuntime(t)
	first := decodeAdminModelsTestList(t, adminModelsTestRequest(r, "/admin/accounts/work/models?pageSize=1", admin))
	if len(first.Models) != 1 || first.Models[0].Name != "models/gemini-work-a" || first.NextPageToken == "" {
		t.Fatalf("first page = %+v", first)
	}
	second := decodeAdminModelsTestList(t, adminModelsTestRequest(r, "/admin/accounts/work/models?pageSize=1&pageToken="+url.QueryEscape(first.NextPageToken), admin))
	if len(second.Models) != 1 || second.Models[0].Name != "models/gemini-work-b" || second.NextPageToken != "" {
		t.Fatalf("second page = %+v", second)
	}
	if response := adminModelsTestRequest(r, "/admin/accounts/work/models?pageSize=-1", admin); response.Code != http.StatusBadRequest {
		t.Fatalf("invalid pagination status = %d", response.Code)
	}
}

func TestAdminAccountModelsAdmissionAndUnavailableStates(t *testing.T) {
	r, admin, workKey, personalKey, workCalls, personalCalls := newAdminModelsTestRuntime(t)
	for _, key := range []string{"", "invalid", workKey, personalKey} {
		if response := adminModelsTestRequest(r, "/admin/accounts/work/models", key); response.Code != http.StatusUnauthorized {
			t.Fatalf("non-admin key models status = %d, want 401", response.Code)
		}
	}
	if response := adminModelsTestRequest(r, "/admin/accounts/work/models?key="+url.QueryEscape(admin), ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("admin key in query status = %d", response.Code)
	}
	for _, tc := range []struct {
		id     string
		status int
	}{{"unknown", http.StatusNotFound}, {"disabled", http.StatusForbidden}, {"unavailable", http.StatusServiceUnavailable}} {
		if response := adminModelsTestRequest(r, "/admin/accounts/"+tc.id+"/models", admin); response.Code != tc.status {
			t.Fatalf("%s models status = %d, want %d", tc.id, response.Code, tc.status)
		}
	}
	if workCalls.Load() != 0 || personalCalls.Load() != 0 {
		t.Fatal("refused admin model requests reached upstream")
	}
	r.registry.Store(nil)
	if response := adminModelsTestRequest(r, "/admin/accounts/work/models", admin); response.Code != http.StatusServiceUnavailable {
		t.Fatalf("unstarted runtime models status = %d", response.Code)
	}
}
