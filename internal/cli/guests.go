// Copyright 2026 sirakinb and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command scaffold. Implement the RunE body before shipping.
// generate --force preserves implemented bodies; untouched TODO scaffolds may refresh.

package cli

import (
	"github.com/spf13/cobra"
)

func newNovelGuestsCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "guests",
		Short:       "Read guest lists and RSVP counts for events you host",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newNovelGuestsSummaryCmd(flags))
	cmd.AddCommand(newGuestsListCmd(flags))
	return cmd
}
