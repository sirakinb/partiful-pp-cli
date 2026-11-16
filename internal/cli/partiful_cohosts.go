// Copyright 2026 sirakinb and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sirakinb/partiful-pp-cli/internal/cliutil"
	"github.com/sirakinb/partiful-pp-cli/internal/partiful"
)

func newEventsCohostsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cohosts",
		Short: "See and add co-hosts on an event",
		RunE:  parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newCohostsListCmd(flags), newCohostsAddCmd(flags))
	return cmd
}

func newCohostsListCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:         "list <event-id-or-url>",
		Short:       "List an event's hosts and co-host requests (pending, accepted, declined)",
		Example:     "  partiful-pp-cli events cohosts list hqWkFzikB1frVdihGKTE",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:happy-args": "<event-id-or-url>=hqWkFzikB1frVdihGKTE"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return dryRunNote(cmd, flags, "would list co-hosts")
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
			reqs, owners, err := pc.Cohosts(ctx, id)
			if err != nil {
				return partifulErr(err)
			}
			return printLive(cmd, flags, map[string]any{"eventId": id, "ownerIds": owners, "cohostRequests": reqs})
		},
	}
}

func newCohostsAddCmd(flags *rootFlags) *cobra.Command {
	var apply bool
	cmd := &cobra.Command{
		Use:   "add <event-id-or-url> <user-id>...",
		Short: "Invite people to co-host an event (preview by default; --apply to send)",
		Long: `Sends a co-host request to each Partiful user id, the same way the "Add co-hosts"
button on partiful.com does. Each person is notified and becomes a co-host once they
accept. Find user ids with "events cohosts list" on an event they already co-host.`,
		Example:     "  partiful-pp-cli events cohosts add jlekt0k9kPDCdSW2HG6Z KPRZOevwecQS6q0tBautP7Q7sb42 --apply",
		Annotations: map[string]string{"pp:method": "POST", "pp:happy-args": "<event-id-or-url>=hqWkFzikB1frVdihGKTE"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return dryRunNote(cmd, flags, "would preview co-host requests")
			}
			if len(args) < 2 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("an event id or URL and at least one user id are required"))
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
			reqs, owners, err := pc.Cohosts(ctx, id)
			if err != nil {
				return partifulErr(err)
			}
			existing := map[string]string{}
			for _, o := range owners {
				existing[o] = "HOST"
			}
			for _, r := range reqs {
				if _, ok := existing[r.UserID]; !ok {
					existing[r.UserID] = r.Status
				}
			}
			toAdd := []string{}
			skipped := map[string]string{}
			for _, u := range args[1:] {
				u = strings.TrimSpace(u)
				if s, ok := existing[u]; ok && s != "DECLINED" {
					skipped[u] = s
					continue
				}
				toAdd = append(toAdd, u)
			}
			plan := map[string]any{"action": "cohosts add", "eventId": id, "url": partiful.EventURL(id), "invite": toAdd,
				"note": "Each person is notified and becomes a co-host after accepting.", "applied": false}
			if len(skipped) > 0 {
				plan["alreadyHostOrInvited"] = skipped
			}
			if len(toAdd) == 0 || !apply || cliutil.IsVerifyEnv() {
				if len(toAdd) > 0 {
					plan["note"] = applyHint
				}
				return flags.printJSON(cmd, plan)
			}
			for _, u := range toAdd {
				if err := pc.RequestCohost(ctx, id, u); err != nil {
					return partifulErr(fmt.Errorf("co-host request for %s: %w", u, err))
				}
			}
			plan["applied"] = true
			if reqs, _, err := pc.Cohosts(ctx, id); err == nil {
				plan["cohostRequests"] = reqs
			}
			return flags.printJSON(cmd, plan)
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "Actually send the co-host requests (default is a preview)")
	return cmd
}

func newEventsQuestionnaireCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "questionnaire",
		Short: "See or copy an event's RSVP questionnaire",
		RunE:  parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newQuestionnaireShowCmd(flags), newQuestionnaireCopyCmd(flags))
	return cmd
}

func newQuestionnaireShowCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:         "show <event-id-or-url>",
		Short:       "Show whether the RSVP questionnaire is on and its current questions",
		Example:     "  partiful-pp-cli events questionnaire show hqWkFzikB1frVdihGKTE",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:happy-args": "<event-id-or-url>=hqWkFzikB1frVdihGKTE"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return dryRunNote(cmd, flags, "would show the questionnaire")
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
			doc, err := pc.EventDocument(ctx, id)
			if err != nil {
				return partifulErr(err)
			}
			enabled, _ := doc.Fields["questionnaireEnabled"].(bool)
			versions, _ := doc.Fields["questionnaireVersions"].([]any)
			return printLive(cmd, flags, map[string]any{"eventId": id, "enabled": enabled, "versions": len(versions), "questions": partiful.LatestQuestions(doc)})
		},
	}
}

func newQuestionnaireCopyCmd(flags *rootFlags) *cobra.Command {
	var apply bool
	cmd := &cobra.Command{
		Use:   "copy <from-event> <to-event>",
		Short: "Copy one event's RSVP questionnaire to another (preview by default; --apply to save)",
		Long: `Saves the source event's current questions (text, type, options, required) as a
new questionnaire version on the target event and turns the questionnaire on, the
same as pressing Save in partiful.com's questionnaire editor.`,
		Example:     "  partiful-pp-cli events questionnaire copy hqWkFzikB1frVdihGKTE jlekt0k9kPDCdSW2HG6Z --apply",
		Annotations: map[string]string{"pp:method": "PATCH", "pp:happy-args": "<from-event>=hqWkFzikB1frVdihGKTE;<to-event>=hqWkFzikB1frVdihGKTE"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return dryRunNote(cmd, flags, "would preview copying a questionnaire")
			}
			if len(args) != 2 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a source and a target event are required"))
			}
			from, err := partiful.ParseEventID(args[0])
			if err != nil {
				return usageErr(err)
			}
			to, err := partiful.ParseEventID(args[1])
			if err != nil {
				return usageErr(err)
			}
			pc, err := newPartiful(flags)
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			src, err := pc.EventDocument(ctx, from)
			if err != nil {
				return partifulErr(err)
			}
			questions := partiful.LatestQuestions(src)
			if len(questions) == 0 {
				return usageErr(fmt.Errorf("event %s has no questionnaire to copy", from))
			}
			dst, err := pc.EventDocument(ctx, to)
			if err != nil {
				return partifulErr(err)
			}
			plan := map[string]any{"action": "questionnaire copy", "from": from, "to": to, "toTitle": dst.Fields["title"],
				"questions": questions, "currentQuestionsOnTarget": len(partiful.LatestQuestions(dst)), "applied": false}
			if !apply || cliutil.IsVerifyEnv() {
				plan["note"] = applyHint
				return flags.printJSON(cmd, plan)
			}
			if err := pc.SetQuestionnaire(ctx, to, questions); err != nil {
				return partifulErr(err)
			}
			plan["applied"] = true
			return flags.printJSON(cmd, plan)
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "Actually save the questionnaire (default is a preview)")
	return cmd
}
