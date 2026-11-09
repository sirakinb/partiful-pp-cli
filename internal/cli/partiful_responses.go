// Copyright 2026 sirakinb and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/csv"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sirakinb/partiful-pp-cli/internal/partiful"
)

func newGuestsResponsesCmd(flags *rootFlags) *cobra.Command {
	var emailsOnly, answeredOnly bool
	var status string
	cmd := &cobra.Command{
		Use:   "responses <event-id-or-url>",
		Short: "Export RSVP questionnaire answers (including emails) for an event you host",
		Long: `Lists each guest's name, RSVP status and answers to the event's RSVP
questionnaire, the same data as the Questionnaire panel on partiful.com.
Only hosts and cohosts can read it.

Answers can contain personal data such as email addresses. Handle the output
with care and don't paste it anywhere public.`,
		Example: strings.Trim(`
  partiful-pp-cli guests responses hqWkFzikB1frVdihGKTE --csv > responses.csv
  partiful-pp-cli guests responses hqWkFzikB1frVdihGKTE --emails
  partiful-pp-cli guests responses hqWkFzikB1frVdihGKTE --answered --status GOING --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:happy-args": "<event-id-or-url>=hqWkFzikB1frVdihGKTE"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return dryRunNote(cmd, flags, "would export questionnaire responses")
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
			questions, responses, err := pc.Questionnaire(ctx, id)
			if err != nil {
				return partifulErr(err)
			}
			filtered := responses[:0:0]
			for _, r := range responses {
				if answeredOnly && !r.Answered {
					continue
				}
				if status != "" && !strings.EqualFold(r.Status, status) {
					continue
				}
				filtered = append(filtered, r)
			}
			if emailsOnly {
				emails := partiful.Emails(filtered)
				if flags.asJSON || flags.csv {
					if flags.csv {
						w := csv.NewWriter(cmd.OutOrStdout())
						_ = w.Write([]string{"email"})
						for _, e := range emails {
							_ = w.Write([]string{e})
						}
						w.Flush()
						return w.Error()
					}
					return flags.printJSON(cmd, emails)
				}
				for _, e := range emails {
					fmt.Fprintln(cmd.OutOrStdout(), e)
				}
				return nil
			}
			if flags.csv {
				return writeResponsesCSV(cmd, questions, filtered)
			}
			return printLive(cmd, flags, filtered)
		},
	}
	cmd.Flags().BoolVar(&emailsOnly, "emails", false, "Print only the unique email addresses guests gave, one per line")
	cmd.Flags().BoolVar(&answeredOnly, "answered", false, "Only guests who answered the questionnaire")
	cmd.Flags().StringVar(&status, "status", "", "Only guests with this RSVP status (GOING, MAYBE, DECLINED, ...)")
	return cmd
}

// writeResponsesCSV writes one row per guest with a column per question, in
// questionnaire order.
func writeResponsesCSV(cmd *cobra.Command, questions []partiful.Question, responses []partiful.QuestionnaireResponse) error {
	w := csv.NewWriter(cmd.OutOrStdout())
	header := []string{"name", "status", "answered"}
	for _, q := range questions {
		header = append(header, q.Text)
	}
	if err := w.Write(header); err != nil {
		return err
	}
	for _, r := range responses {
		byID := make(map[string]string, len(r.Answers))
		for _, a := range r.Answers {
			byID[a.QuestionID] = a.Answer
		}
		row := []string{r.Name, r.Status, fmt.Sprint(r.Answered)}
		for _, q := range questions {
			row = append(row, byID[q.ID])
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}
