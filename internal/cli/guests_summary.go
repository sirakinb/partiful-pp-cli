// Copyright 2026 sirakinb and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sirakinb/partiful-pp-cli/internal/partiful"
)

// pp:data-source live
func newNovelGuestsSummaryCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "summary <event-id-or-url>",
		Short:       "See going, maybe, declined and waitlist counts for an event you host, with plus-ones.",
		Example:     "  partiful-pp-cli guests summary hqWkFzikB1frVdihGKTE --json",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:happy-args": "<event-id-or-url>=hqWkFzikB1frVdihGKTE"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return dryRunNote(cmd, flags, "would summarize guests")
			}
			if len(args) != 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("an event id or URL is required"))
			}
			id, err := partiful.ParseEventID(args[0])
			if err != nil {
				return usageErr(err)
			}
			pc, err := newPartiful(flags)
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			guests, err := pc.ListGuests(ctx, id)
			if err != nil {
				return partifulErr(err)
			}
			return printLive(cmd, flags, partiful.Summarize(id, guests))
		},
	}
	return cmd
}
