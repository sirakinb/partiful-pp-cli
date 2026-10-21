// Copyright 2026 sirakinb and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sirakinb/partiful-pp-cli/internal/partiful"
)

func newGuestsListCmd(flags *rootFlags) *cobra.Command {
	var status string
	cmd := &cobra.Command{
		Use:   "list <event-id-or-url>",
		Short: "List guests and RSVPs for an event you host",
		Example: strings.Trim(`
  partiful-pp-cli guests list hqWkFzikB1frVdihGKTE
  partiful-pp-cli guests list hqWkFzikB1frVdihGKTE --status GOING --csv`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:happy-args": "<event-id-or-url>=hqWkFzikB1frVdihGKTE"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return dryRunNote(cmd, flags, "would list guests")
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
			if status != "" {
				want := strings.ToUpper(status)
				filtered := guests[:0:0]
				for _, g := range guests {
					if g.Status == want {
						filtered = append(filtered, g)
					}
				}
				guests = filtered
			}
			return printLive(cmd, flags, guests)
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "Only guests with this RSVP status (GOING, MAYBE, DECLINED, WAITLIST, ...)")
	return cmd
}
