// Copyright 2026 sirakinb and contributors. Licensed under Apache-2.0. See LICENSE.

// Package partiful is the hand-written client for Partiful's private web API.
//
// Partiful has no public API. Its web app talks to three surfaces, all
// authenticated with a Firebase ID token for the signed-in user:
//
//   - Firebase callable functions at https://api.partiful.com/<name>
//   - Firestore REST for project "getpartiful" (event docs, host guest lists)
//   - Firebase Storage bucket getpartiful.appspot.com (event images)
//
// The CLI stores only the long-lived Firebase refresh token (plus a cached,
// short-lived ID token) in a 0600 file under the user's config directory and
// mints fresh ID tokens from it on demand.
package partiful

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirakinb/partiful-pp-cli/internal/cliutil"
)

// RefreshTokenEnv lets callers supply the refresh token without a session file
// (CI, one-off agent runs). It wins over the stored session.
const RefreshTokenEnv = "PARTIFUL_REFRESH_TOKEN"

// SessionPathEnv overrides where the session file lives.
const SessionPathEnv = "PARTIFUL_SESSION_FILE"

// ErrNoSession is returned when neither a session file nor the env var exists.
var ErrNoSession = errors.New("not signed in: run `partiful-pp-cli auth login` (see README for how to copy your session from the browser)")

// Session is the on-disk auth state. RefreshToken is the secret; IDToken is a
// cache that expires after about an hour.
type Session struct {
	RefreshToken string    `json:"refresh_token"`
	UserID       string    `json:"user_id,omitempty"`
	IDToken      string    `json:"id_token,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
	SavedAt      time.Time `json:"saved_at,omitempty"`
	fromEnv      bool
	path         string
}

// FromEnv reports whether the session came from PARTIFUL_REFRESH_TOKEN.
func (s *Session) FromEnv() bool { return s.fromEnv }

// Path is the file the session was loaded from or will be saved to.
func (s *Session) Path() string { return s.path }

// DefaultSessionPath returns <config dir>/session.json (normally
// ~/.config/partiful-pp-cli; PARTIFUL_HOME and PARTIFUL_CONFIG_DIR relocate it)
// unless PARTIFUL_SESSION_FILE is set.
func DefaultSessionPath() (string, error) {
	if p := strings.TrimSpace(os.Getenv(SessionPathEnv)); p != "" {
		return p, nil
	}
	dir, err := cliutil.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "session.json"), nil
}

// LoadSession reads the session from the env var or the session file.
func LoadSession() (*Session, error) {
	path, err := DefaultSessionPath()
	if err != nil {
		return nil, err
	}
	if tok := strings.TrimSpace(os.Getenv(RefreshTokenEnv)); tok != "" {
		return &Session{RefreshToken: tok, fromEnv: true, path: path}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNoSession
		}
		return nil, err
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	if s.RefreshToken == "" {
		return nil, ErrNoSession
	}
	s.path = path
	return &s, nil
}

// Save writes the session with owner-only permissions. Env-sourced sessions
// are never persisted.
func (s *Session) Save() error {
	if s.fromEnv {
		return nil
	}
	if s.path == "" {
		p, err := DefaultSessionPath()
		if err != nil {
			return err
		}
		s.path = p
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// DeleteSession removes the session file. Missing files are not an error.
func DeleteSession() (string, error) {
	path, err := DefaultSessionPath()
	if err != nil {
		return "", err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return path, err
	}
	return path, nil
}

// NewSession builds an unsaved session for a refresh token.
func NewSession(refreshToken string) (*Session, error) {
	path, err := DefaultSessionPath()
	if err != nil {
		return nil, err
	}
	return &Session{RefreshToken: strings.TrimSpace(refreshToken), path: path, SavedAt: time.Now().UTC()}, nil
}
