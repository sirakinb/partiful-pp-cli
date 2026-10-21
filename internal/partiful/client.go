// Copyright 2026 sirakinb and contributors. Licensed under Apache-2.0. See LICENSE.

package partiful

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/sirakinb/partiful-pp-cli/internal/cliutil"
)

const (
	// webAPIKey is the public Firebase web API key embedded in partiful.com's
	// JavaScript bundle. It identifies the Firebase project; it is not a
	// secret and grants nothing without a user's refresh token.
	webAPIKey = "AIzaSyCky6PJ7cHRdBKk5X7gjuWERWaKWBHr4_k"
	projectID = "getpartiful"
	bucket    = "getpartiful.appspot.com"

	defaultAPIBase       = "https://api.partiful.com"
	defaultFirestoreBase = "https://firestore.googleapis.com/v1/projects/" + projectID + "/databases/(default)/documents"
	defaultStorageBase   = "https://firebasestorage.googleapis.com/v0/b/" + bucket + "/o"
	defaultTokenURL      = "https://securetoken.googleapis.com/v1/token"

	webOrigin = "https://partiful.com"
)

// APIError is a non-2xx response or a callable-level error payload.
type APIError struct {
	Status  int
	Code    string
	Message string
	Surface string
}

func (e *APIError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = http.StatusText(e.Status)
	}
	if e.Code != "" {
		return fmt.Sprintf("%s: %s (%s, HTTP %d)", e.Surface, msg, e.Code, e.Status)
	}
	return fmt.Sprintf("%s: %s (HTTP %d)", e.Surface, msg, e.Status)
}

// IsAuth reports whether the error means the session is missing or rejected.
func (e *APIError) IsAuth() bool {
	return e.Status == http.StatusUnauthorized || e.Code == "UNAUTHENTICATED"
}

// IsNotFound reports a missing resource.
func (e *APIError) IsNotFound() bool {
	return e.Status == http.StatusNotFound || e.Code == "NOT_FOUND"
}

// IsPermission reports a rules/permission rejection.
func (e *APIError) IsPermission() bool {
	return e.Status == http.StatusForbidden || e.Code == "PERMISSION_DENIED"
}

// Client talks to Partiful as the signed-in user.
type Client struct {
	HTTP          *http.Client
	Session       *Session
	APIBase       string
	FirestoreBase string
	StorageBase   string
	TokenURL      string
	limiter       *cliutil.AdaptiveLimiter
}

// New loads the saved session and returns a client. Base URLs can be pointed
// at test servers with PARTIFUL_BASE_URL (callables only).
func New(timeout time.Duration) (*Client, error) {
	s, err := LoadSession()
	if err != nil {
		return nil, err
	}
	return NewWithSession(s, timeout), nil
}

// NewWithSession builds a client around an explicit session.
func NewWithSession(s *Session, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	apiBase := defaultAPIBase
	if v := strings.TrimRight(os.Getenv("PARTIFUL_BASE_URL"), "/"); v != "" {
		apiBase = v
	}
	return &Client{
		HTTP:          &http.Client{Timeout: timeout},
		Session:       s,
		APIBase:       apiBase,
		FirestoreBase: defaultFirestoreBase,
		StorageBase:   defaultStorageBase,
		TokenURL:      defaultTokenURL,
		limiter:       cliutil.NewAdaptiveLimiter(4),
	}
}

// UserID returns the signed-in Firebase uid, refreshing the token if needed.
func (c *Client) UserID(ctx context.Context) (string, error) {
	if _, err := c.IDToken(ctx); err != nil {
		return "", err
	}
	return c.Session.UserID, nil
}

// IDToken returns a valid ID token, exchanging the refresh token when the
// cached one is missing or within two minutes of expiry.
func (c *Client) IDToken(ctx context.Context) (string, error) {
	s := c.Session
	if s.IDToken != "" && s.UserID != "" && time.Until(s.ExpiresAt) > 2*time.Minute {
		return s.IDToken, nil
	}
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {s.RefreshToken}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.TokenURL+"?key="+webAPIKey, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	setBrowserHeaders(req)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("refreshing Partiful session: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(body, &e)
		return "", &APIError{Status: http.StatusUnauthorized, Code: "UNAUTHENTICATED", Surface: "token refresh",
			Message: "session rejected (" + e.Error.Message + "); sign in again with `partiful-pp-cli auth login`"}
	}
	var tok struct {
		IDToken      string `json:"id_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    string `json:"expires_in"`
		UserID       string `json:"user_id"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", fmt.Errorf("parsing token response: %w", err)
	}
	secs := 3600
	fmt.Sscanf(tok.ExpiresIn, "%d", &secs)
	s.IDToken = tok.IDToken
	s.UserID = tok.UserID
	s.ExpiresAt = time.Now().Add(time.Duration(secs) * time.Second).UTC()
	if tok.RefreshToken != "" {
		s.RefreshToken = tok.RefreshToken
	}
	if err := s.Save(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not cache Partiful session: %v\n", err)
	}
	return s.IDToken, nil
}

func setBrowserHeaders(req *http.Request) {
	req.Header.Set("Origin", webOrigin)
	req.Header.Set("Referer", webOrigin+"/")
	req.Header.Set("User-Agent", "github.com/sirakinb/partiful-pp-cli/0.1 (+https://partiful.com)")
}

func (c *Client) do(ctx context.Context, surface, method, rawURL string, body io.Reader, contentType string, authScheme string) ([]byte, error) {
	tok, err := c.IDToken(ctx)
	if err != nil {
		return nil, err
	}
	var payload []byte
	if body != nil {
		if payload, err = io.ReadAll(body); err != nil {
			return nil, err
		}
	}
	const maxAttempts = 3
	for attempt := 0; ; attempt++ {
		c.limiter.Wait()
		req, err := http.NewRequestWithContext(ctx, method, rawURL, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		req.Header.Set("Authorization", authScheme+" "+tok)
		setBrowserHeaders(req)
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", surface, err)
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			c.limiter.OnRateLimit()
			wait := cliutil.RetryAfter(resp)
			if attempt+1 >= maxAttempts {
				return nil, &cliutil.RateLimitError{URL: surface, RetryAfter: wait, Body: string(data)}
			}
			if wait <= 0 {
				wait = cliutil.Backoff(attempt)
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
			continue
		}
		c.limiter.OnSuccess()
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return nil, parseError(surface, resp.StatusCode, data)
		}
		return data, nil
	}
}

func parseError(surface string, status int, data []byte) error {
	var e struct {
		Error struct {
			Message string `json:"message"`
			Status  string `json:"status"`
			Code    any    `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &e) == nil && (e.Error.Message != "" || e.Error.Status != "") {
		return &APIError{Status: status, Code: e.Error.Status, Message: e.Error.Message, Surface: surface}
	}
	msg := strings.TrimSpace(string(data))
	if len(msg) > 200 {
		msg = msg[:200]
	}
	return &APIError{Status: status, Message: msg, Surface: surface}
}

// Call invokes a callable function. payload is merged into the callable's
// "data" object (typically {"params": {...}}); userId is added automatically.
// It returns result.data.
func (c *Client) Call(ctx context.Context, function string, payload map[string]any) (json.RawMessage, error) {
	uid, err := c.UserID(ctx)
	if err != nil {
		return nil, err
	}
	data := map[string]any{}
	for k, v := range payload {
		data[k] = v
	}
	if _, ok := data["params"]; !ok {
		data["params"] = map[string]any{}
	}
	data["userId"] = uid
	buf, err := json.Marshal(map[string]any{"data": data})
	if err != nil {
		return nil, err
	}
	raw, err := c.do(ctx, function, http.MethodPost, c.APIBase+"/"+url.PathEscape(function), bytes.NewReader(buf), "application/json", "Bearer")
	if err != nil {
		return nil, err
	}
	var env struct {
		Result *struct {
			Data json.RawMessage `json:"data"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
			Status  string `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("%s: unexpected response: %w", function, err)
	}
	if env.Error != nil {
		return nil, &APIError{Status: http.StatusBadRequest, Code: env.Error.Status, Message: env.Error.Message, Surface: function}
	}
	if env.Result == nil {
		return json.RawMessage("null"), nil
	}
	return env.Result.Data, nil
}
