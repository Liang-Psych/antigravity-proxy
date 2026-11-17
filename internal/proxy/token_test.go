package proxy

import (
	"antigravity-proxy/internal/config"
	"antigravity-proxy/internal/quota"
	"antigravity-proxy/internal/testutil"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func waitTokenRefreshWaiters(t *testing.T, source *tokenSource, count int) {
	t.Helper()
	testutil.Eventually(t, func() bool {
		source.mu.Lock()
		defer source.mu.Unlock()
		return source.flight != nil && source.flight.waiters == count
	}, "token callers did not join shared refresh")
}

func TestTokenRefreshWaiterDeadline(t *testing.T) {
	for _, rejected := range []bool{false, true} {
		name := "expired"
		if rejected {
			name = "rejected"
		}
		t.Run(name, func(t *testing.T) {
			started := make(chan struct{})
			release := make(chan struct{})
			p := recoveryProxy(t, "http://unused.invalid", func(w http.ResponseWriter, r *http.Request) {
				close(started)
				<-release
				_, _ = io.WriteString(w, `{"access_token":"new-token","expires_in":3600}`)
			})
			p.tokens.seed("old-token", time.Now().Add(-time.Hour))
			fetch := func(ctx context.Context) (string, error) {
				if rejected {
					return p.tokens.refreshRejected(ctx, "old-token", false)
				}
				return p.accessToken(ctx)
			}
			owner := make(chan error, 1)
			go func() {
				_, err := fetch(context.Background())
				owner <- err
			}()
			t.Cleanup(func() {
				close(release)
				if err := <-owner; err != nil {
					t.Errorf("refresh owner failed: %v", err)
				}
			})
			awaitSurface(t, started)
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			waiter := make(chan error, 1)
			go func() {
				_, err := fetch(ctx)
				waiter <- err
			}()
			select {
			case err := <-waiter:
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("waiter error = %v; want deadline exceeded", err)
				}
			case <-time.After(time.Second):
				t.Fatal("canceled waiter remained blocked behind the OAuth refresh")
			}
		})
	}
}

func TestTokenRefreshInitiatorCancelsIndependently(t *testing.T) {
	var calls atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	p := recoveryProxy(t, "http://unused.invalid", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		close(started)
		<-release
		_, _ = io.WriteString(w, `{"access_token":"new-token","expires_in":3600}`)
	})
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	initiator := make(chan error, 1)
	go func() {
		_, err := p.tokens.refreshRejected(ctx, "old-token", false)
		initiator <- err
	}()
	awaitSurface(t, started)
	waiter := make(chan error, 1)
	go func() {
		token, err := p.accessToken(context.Background())
		if err == nil && token != "new-token" {
			err = errors.New("waiter received the rejected token")
		}
		waiter <- err
	}()
	waitTokenRefreshWaiters(t, p.tokens, 2)
	cancel()
	if err := testutil.Await(t, initiator, "refresh initiator ignored cancellation"); !errors.Is(err, context.Canceled) {
		t.Fatalf("initiator error = %v", err)
	}
	if _, err := p.quotaAccessToken(context.Background()); !errors.Is(err, quota.ErrAuthenticationBusy) {
		t.Fatalf("quota poll waited for shared refresh: %v", err)
	}
	// Runtime identity promotion must be able to snapshot during network I/O.
	if token, _ := p.tokens.snapshot(); token != "" {
		t.Fatal("snapshot retained a rejected token")
	}
	releaseOnce.Do(func() { close(release) })
	if err := testutil.Await(t, waiter, "surviving waiter did not finish"); err != nil {
		t.Fatal(err)
	}
	if token, err := p.quotaAccessToken(context.Background()); err != nil || token != "new-token" || calls.Load() != 1 {
		t.Fatalf("shared refresh was not cached: error=%v calls=%d", err, calls.Load())
	}
}

func TestTokenRefreshAbandonedResultCannotOverwriteReplacement(t *testing.T) {
	started, canceled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	var calls atomic.Int32
	source := &tokenSource{
		cfg: &config.Config{RefreshToken: "test-refresh", OAuthClientID: "test-client", OAuthClientSecret: "test-secret"},
		client: &http.Client{Transport: testutil.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
			token := "replacement-token"
			if calls.Add(1) == 1 {
				close(started)
				<-r.Context().Done()
				close(canceled)
				<-release
				token = "abandoned-token"
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"access_token":"` + token + `","expires_in":3600}`)), Header: make(http.Header)}, nil
		})},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := source.token(ctx, false)
		result <- err
	}()
	awaitSurface(t, started)
	source.mu.Lock()
	abandoned := source.flight
	source.mu.Unlock()
	cancel()
	if err := testutil.Await(t, result, "last waiter ignored cancellation"); !errors.Is(err, context.Canceled) {
		t.Fatalf("last waiter error = %v", err)
	}
	awaitSurface(t, canceled)
	if token, err := source.token(context.Background(), false); err != nil || token != "replacement-token" {
		t.Fatalf("replacement refresh failed: %v", err)
	}
	releaseOnce.Do(func() { close(release) })
	awaitSurface(t, abandoned.done)
	if token, _ := source.snapshot(); token != "replacement-token" || calls.Load() != 2 {
		t.Fatalf("abandoned refresh overwrote replacement; calls=%d", calls.Load())
	}
}

func TestTokenRefreshSharesFailureAndAllowsRetry(t *testing.T) {
	var calls atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	p := recoveryProxy(t, "http://unused.invalid", func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			close(started)
			<-release
			http.Error(w, "secret-provider-error", http.StatusBadRequest)
			return
		}
		_, _ = io.WriteString(w, `{"access_token":"retry-token","expires_in":3600}`)
	})
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	p.tokens.seed("old-token", time.Now().Add(-time.Hour))
	const waiters = 12
	results := make(chan error, waiters)
	for i := range waiters {
		go func() {
			var err error
			if i%2 == 0 {
				_, err = p.accessToken(context.Background())
			} else {
				_, err = p.tokens.refreshRejected(context.Background(), "old-token", false)
			}
			results <- err
		}()
	}
	awaitSurface(t, started)
	waitTokenRefreshWaiters(t, p.tokens, waiters)
	releaseOnce.Do(func() { close(release) })
	for range waiters {
		if err := testutil.Await(t, results, "shared refresh failure did not reach waiter"); err == nil || err.Error() != "Google OAuth refresh failed" {
			t.Fatalf("unexpected or unsanitized refresh error: %v", err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("concurrent refresh failure issued %d OAuth requests", calls.Load())
	}
	if token, _ := p.tokens.snapshot(); token != "" {
		t.Fatal("failed refresh retained an expired token")
	}
	if token, err := p.accessToken(context.Background()); err != nil || token != "retry-token" || calls.Load() != 2 {
		t.Fatalf("retry after shared failure: error=%v calls=%d", err, calls.Load())
	}
}
