// Copyright 2026 sirakinb and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/sirakinb/partiful-pp-cli/internal/client"
	"github.com/sirakinb/partiful-pp-cli/internal/cliutil"
	"github.com/sirakinb/partiful-pp-cli/internal/partiful"
)

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		root.AddCommand(newAuthCmd(flags))
		root.AddCommand(newEventsCmd(flags))
	})
	// The generated `functions` escape hatch uses the generated client; give
	// it a fresh ID token from the saved session when one exists.
	registerClientHook(func(c *client.Client) error {
		if c.DryRun || cliutil.IsVerifyEnv() {
			return nil
		}
		pc, err := partiful.New(30 * time.Second)
		if err != nil {
			return nil
		}
		tok, err := pc.IDToken(context.Background())
		if err != nil {
			return nil
		}
		c.Config.AuthHeaderVal = "Bearer " + tok
		return nil
	})
}

// newPartiful builds the hand-written client, mapping auth problems to exit 4.
func newPartiful(flags *rootFlags) (*partiful.Client, error) {
	c, err := partiful.New(flags.timeout)
	if err != nil {
		if errors.Is(err, partiful.ErrNoSession) {
			return nil, authErr(err)
		}
		return nil, configErr(err)
	}
	return c, nil
}

// partifulErr maps client errors onto the CLI's typed exit codes.
func partifulErr(err error) error {
	var ae *partiful.APIError
	if errors.As(err, &ae) {
		switch {
		case ae.IsAuth():
			return authErr(err)
		case ae.IsNotFound():
			return notFoundErr(err)
		case ae.IsPermission():
			return apiErr(fmt.Errorf("%w\nhint: guest lists and edits are only available for events you host or cohost", err))
		default:
			return apiErr(err)
		}
	}
	var rl *cliutil.RateLimitError
	if errors.As(err, &rl) {
		return rateLimitErr(err)
	}
	if errors.Is(err, partiful.ErrNoSession) {
		return authErr(err)
	}
	return err
}

func newAuthCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage your saved Partiful session",
		Long: `Partiful has no API keys. This CLI uses the Firebase refresh token from your own
signed-in browser session. The token is stored in ~/.config/partiful-pp-cli/session.json
with 0600 permissions (override with PARTIFUL_SESSION_FILE), or read from
PARTIFUL_REFRESH_TOKEN. See the README for the one-line browser snippet that copies it.`,
		RunE: parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newAuthLoginCmd(flags), newAuthStatusCmd(flags), newAuthLogoutCmd(flags))
	return cmd
}

func newAuthLoginCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "login",
		Short:       "Save your Partiful session (Firebase refresh token) locally",
		Annotations: map[string]string{"pp:method": "POST", "mcp:hidden": "true"},
		Long: `Reads a Firebase refresh token from stdin (paste it and press Enter, or pipe it),
verifies it with Google's token service, and saves it with 0600 permissions.

To copy the token: open partiful.com signed in, open DevTools > Console, and run the
snippet from the README. It copies the token to your clipboard without printing it.`,
		Example: strings.Trim(`
  pbpaste | partiful-pp-cli auth login
  partiful-pp-cli auth login`, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) || cliutil.IsVerifyEnv() {
				return dryRunNote(cmd, flags, "would read a refresh token from stdin and save it")
			}
			in := cmd.InOrStdin()
			if f, ok := in.(*os.File); ok && isTerminal(f) {
				fmt.Fprint(cmd.ErrOrStderr(), "Paste your Partiful refresh token and press Enter: ")
			}
			line, err := bufio.NewReader(in).ReadString('\n')
			if err != nil && !errors.Is(err, io.EOF) {
				return err
			}
			token := strings.Trim(strings.TrimSpace(line), `"'`)
			if token == "" {
				return usageErr(fmt.Errorf("no token read from stdin"))
			}
			s, err := partiful.NewSession(token)
			if err != nil {
				return configErr(err)
			}
			pc := partiful.NewWithSession(s, flags.timeout)
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			if _, err := pc.IDToken(ctx); err != nil {
				return partifulErr(err)
			}
			if err := s.Save(); err != nil {
				return configErr(err)
			}
			return flags.printJSON(cmd, map[string]any{"signedIn": true, "userId": s.UserID, "sessionFile": s.Path()})
		},
	}
	return cmd
}

func newAuthStatusCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:         "status",
		Short:       "Show whether a Partiful session is saved and still valid",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			s, err := partiful.LoadSession()
			if err != nil {
				_ = flags.printJSON(cmd, map[string]any{"signedIn": false, "reason": err.Error()})
				return authErr(err)
			}
			pc := partiful.NewWithSession(s, flags.timeout)
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			if _, err := pc.IDToken(ctx); err != nil {
				_ = flags.printJSON(cmd, map[string]any{"signedIn": false, "reason": err.Error()})
				return partifulErr(err)
			}
			src := s.Path()
			if s.FromEnv() {
				src = "$" + partiful.RefreshTokenEnv
			}
			return flags.printJSON(cmd, map[string]any{"signedIn": true, "userId": s.UserID, "source": src, "idTokenExpiresAt": s.ExpiresAt})
		},
	}
}

func newAuthLogoutCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:         "logout",
		Short:       "Remove the saved Partiful session",
		Annotations: map[string]string{"pp:method": "DELETE", "mcp:hidden": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) || cliutil.IsVerifyEnv() || cliutil.IsDogfoodEnv() {
				return dryRunNote(cmd, flags, "would delete the saved session file")
			}
			path, err := partiful.DeleteSession()
			if err != nil {
				return configErr(err)
			}
			return flags.printJSON(cmd, map[string]any{"signedIn": false, "removed": path})
		},
	}
}
