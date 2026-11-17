package proxy

import (
	"antigravity-proxy/internal/config"
	"antigravity-proxy/internal/quota"
	"antigravity-proxy/internal/testutil"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func recoveryProxy(t *testing.T, upstream string, oauthHandler http.HandlerFunc) *Proxy {
	t.Helper()
	server := httptest.NewServer(oauthHandler)
	t.Cleanup(server.Close)
	p := newProxy(config.Config{RefreshToken: "test-refresh", OAuthClientID: "test-client", OAuthClientSecret: "test-secret", DailyEndpoint: upstream, ProdEndpoint: upstream}, nil, nil, testutil.RedirectClient(t, server.URL))
	p.tokens.seed("old-token", time.Now().Add(time.Hour))
	return p
}

func TestUpstreamOAuthRecovery(t *testing.T) {
	for _, mode := range []string{"success", "second-401", "refresh-failure", "explicit-token", "forbidden", "stream-truncated"} {
		t.Run(mode, func(t *testing.T) {
			var requests, refreshes atomic.Int32
			const body = `{"request":{"unknown":9007199254740993}}`
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := requests.Add(1)
				got, _ := io.ReadAll(r.Body)
				if string(got) != body {
					t.Error("replay changed request bytes")
				}
				if mode == "stream-truncated" {
					io.WriteString(w, "data: {\"response\":")
					return
				}
				if mode == "forbidden" {
					w.WriteHeader(403)
					return
				}
				if n == 1 || mode == "second-401" {
					w.WriteHeader(401)
					return
				}
				if r.Header.Get("Authorization") != "Bearer new-token" {
					t.Error("replacement token not used")
				}
				io.WriteString(w, "{}")
			}))
			defer upstream.Close()
			p := recoveryProxy(t, upstream.URL, func(w http.ResponseWriter, r *http.Request) {
				refreshes.Add(1)
				if mode == "refresh-failure" {
					w.WriteHeader(400)
					io.WriteString(w, "secret-provider-message")
					return
				}
				io.WriteString(w, `{"access_token":"new-token","expires_in":3600}`)
			})
			if mode == "explicit-token" {
				p.cfg.AccessToken = "old-token"
			}
			resp, err := p.post(context.Background(), "old-token", jsonUpstreamCall("/generate", "application/json", "", []byte(body)))
			if mode == "success" || mode == "stream-truncated" {
				if err != nil {
					t.Fatal(err)
				}
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			} else if err == nil {
				resp.Body.Close()
				t.Fatal("expected failure")
			}
			wantRequests, wantRefreshes := int32(1), int32(0)
			switch mode {
			case "success", "second-401":
				wantRequests, wantRefreshes = 2, 1
			case "refresh-failure":
				wantRefreshes = 1
			}
			if requests.Load() != wantRequests || refreshes.Load() != wantRefreshes {
				t.Fatalf("requests=%d refreshes=%d", requests.Load(), refreshes.Load())
			}
			if mode == "refresh-failure" {
				if strings.Contains(err.Error(), "secret") {
					t.Fatal("provider error leaked")
				}
				token, _ := p.tokens.snapshot()
				if token != "" {
					t.Fatal("rejected credential retained")
				}
			}
		})
	}
}

func TestConcurrentOAuthRecoverySharesReplacement(t *testing.T) {
	var refreshes atomic.Int32
	p := recoveryProxy(t, "http://unused.invalid", func(w http.ResponseWriter, r *http.Request) {
		refreshes.Add(1)
		io.WriteString(w, `{"access_token":"new-token","expires_in":3600}`)
	})
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			token, err := p.tokens.refreshRejected(context.Background(), "old-token", false)
			if err != nil || token != "new-token" {
				t.Errorf("recovery failed: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()
	if refreshes.Load() != 1 {
		t.Fatalf("OAuth refreshed %d times", refreshes.Load())
	}
}

func TestOAuthRecoveryCancellationAndBusyPoll(t *testing.T) {
	p := recoveryProxy(t, "http://unused.invalid", func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected OAuth request") })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.tokens.refreshRejected(ctx, "old-token", false); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error: %v", err)
	}
	p.tokens.mu.Lock()
	defer p.tokens.mu.Unlock()
	if _, err := p.tokens.refreshRejected(context.Background(), "old-token", true); !errors.Is(err, quota.ErrAuthenticationBusy) {
		t.Fatalf("poll error: %v", err)
	}
}

func TestOAuthRecoveryBudgetSpansEndpoints(t *testing.T) {
	var dailyCalls, prodCalls, refreshes atomic.Int32
	daily := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if dailyCalls.Add(1) == 1 {
			w.WriteHeader(401)
		} else {
			w.WriteHeader(503)
		}
	}))
	defer daily.Close()
	prod := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		prodCalls.Add(1)
		if r.Header.Get("Authorization") != "Bearer new-token" {
			t.Error("fallback lost replacement token")
		}
		w.WriteHeader(401)
	}))
	defer prod.Close()
	p := recoveryProxy(t, daily.URL, func(w http.ResponseWriter, r *http.Request) {
		refreshes.Add(1)
		io.WriteString(w, `{"access_token":"new-token","expires_in":3600}`)
	})
	p.cfg.ProdEndpoint = prod.URL
	response, err := p.postJSON(context.Background(), "old-token", "/operation", "", nil)
	if response != nil {
		defer response.Body.Close()
	}
	var rejected *upstreamError
	if !errors.As(err, &rejected) || rejected.Status != 401 {
		t.Fatalf("expected final 401: %v", err)
	}
	if dailyCalls.Load() != 2 || prodCalls.Load() != 1 || refreshes.Load() != 1 {
		t.Fatalf("daily=%d prod=%d refresh=%d", dailyCalls.Load(), prodCalls.Load(), refreshes.Load())
	}
}
