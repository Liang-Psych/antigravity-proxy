package proxy

import (
	"antigravity-proxy/internal/config"
	"antigravity-proxy/internal/oauth"
	"antigravity-proxy/internal/quota"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// tokenSource owns refresh serialization and token lifetime. cfg and client are
// immutable; mu protects the cache and shared refresh flight, never network I/O.
// Account promotion uses snapshot/seed rather than sharing runtime snapshot locks.
type tokenSource struct {
	cfg    *config.Config
	client *http.Client

	mu          sync.Mutex
	cachedToken string
	expiresAt   time.Time
	flight      *sharedFlight[string]
}

func (s *tokenSource) seed(token string, expires time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cachedToken, s.expiresAt = token, expires
}

func (s *tokenSource) snapshot() (string, time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cachedToken, s.expiresAt
}

// token uses an explicit access token unchanged. Polls never wait for an
// unrelated refresh owner: busy polls retry on a later scheduled attempt.
func (s *tokenSource) token(ctx context.Context, poll bool) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if s.cfg.AccessToken != "" {
		return s.cfg.AccessToken, nil
	}
	if s.cfg.RefreshToken == "" {
		return "", errors.New("no Google credential configured; use /admin/login-sessions or configure credentials")
	}
	return s.tokenOrRefresh(ctx, "", poll)
}

// refreshRejected serializes recovery and reuses a replacement already obtained
// by another request. Explicit access tokens are never replaced by OAuth.
func (s *tokenSource) refreshRejected(ctx context.Context, rejected string, poll bool) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return s.tokenOrRefresh(ctx, rejected, poll)
}

func (s *tokenSource) tokenOrRefresh(ctx context.Context, rejected string, poll bool) (string, error) {
	if poll {
		if !s.mu.TryLock() {
			return "", quota.ErrAuthenticationBusy
		}
	} else {
		s.mu.Lock()
	}
	if err := ctx.Err(); err != nil {
		s.mu.Unlock()
		return "", err
	}
	if s.cachedToken != rejected && s.cachedToken != "" && time.Until(s.expiresAt) > time.Minute {
		token := s.cachedToken
		s.mu.Unlock()
		return token, nil
	}
	flight := s.flight
	if poll && flight != nil {
		s.mu.Unlock()
		return "", quota.ErrAuthenticationBusy
	}
	var refreshCtx context.Context
	if flight == nil {
		// A rejected token must not be reused even if this refresh fails.
		s.cachedToken, s.expiresAt = "", time.Time{}
		var cancel context.CancelFunc
		refreshCtx, cancel = context.WithTimeout(context.WithoutCancel(ctx), oauth.DefaultHTTPTimeout)
		flight = newSharedFlight[string](cancel)
		s.flight = flight
	}
	flight.join()
	s.mu.Unlock()
	if refreshCtx != nil {
		go s.refreshFlight(refreshCtx, flight)
	}
	return flight.wait(ctx, &s.mu, func() {
		if s.flight == flight {
			s.flight = nil
		}
	})
}

func (s *tokenSource) refreshFlight(ctx context.Context, flight *sharedFlight[string]) {
	defer flight.cancel()
	tokens, err := s.refresh(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	// An abandoned refresh must not overwrite a newer flight's cache.
	if s.flight == flight {
		s.flight = nil
		if err == nil && ctx.Err() == nil {
			s.cachedToken, s.expiresAt = tokens.AccessToken, tokens.Expiry(time.Now())
		}
	}
	flight.finish(tokens.AccessToken, err)
}

// refresh retains only sanitized errors; waiters keep their own cancellation.
func (s *tokenSource) refresh(ctx context.Context) (oauth.Tokens, error) {
	tokens, err := oauth.RefreshWithClient(ctx, *s.cfg, s.client)
	if ctx.Err() != nil {
		return oauth.Tokens{}, ctx.Err()
	}
	if err != nil {
		slog.Warn("upstream authentication failed", "operation", "oauth_refresh")
		return oauth.Tokens{}, errors.New("Google OAuth refresh failed")
	}
	if tokens.AccessToken == "" {
		return oauth.Tokens{}, errors.New("Google OAuth refresh response contained no access token")
	}
	return tokens, nil
}

func (p *Proxy) accessToken(ctx context.Context) (string, error) {
	return p.tokens.token(ctx, false)
}

func (p *Proxy) quotaAccessToken(ctx context.Context) (string, error) {
	return p.tokens.token(ctx, true)
}
