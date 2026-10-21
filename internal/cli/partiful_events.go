// Copyright 2026 sirakinb and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/sirakinb/partiful-pp-cli/internal/cliutil"
	"github.com/sirakinb/partiful-pp-cli/internal/partiful"
)

const applyHint = "Preview only, nothing was changed. Re-run with --apply to make this change."

func newEventsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "events",
		Short: "List, read, create, and edit your Partiful events",
		RunE:  parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newEventsListCmd(flags), newEventsGetCmd(flags), newEventsCreateCmd(flags),
		newEventsUpdateCmd(flags), newEventsPublishCmd(flags))
	return cmd
}

// dryRunNote reports what a command would do without calling Partiful,
// as JSON when --json/--agent is set so callers always get parseable output.
func dryRunNote(cmd *cobra.Command, flags *rootFlags, msg string) error {
	if flags.asJSON {
		return flags.printJSON(cmd, map[string]any{"dryRun": true, "wouldDo": strings.TrimPrefix(msg, "would ")})
	}
	fmt.Fprintln(cmd.OutOrStdout(), msg)
	return nil
}

func printLive(cmd *cobra.Command, flags *rootFlags, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return printOutputWithFlagsMeta(cmd.OutOrStdout(), data, flags, map[string]any{"source": "live"})
}

func newEventsListCmd(flags *rootFlags) *cobra.Command {
	var past, all, hosting bool
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List your events (upcoming by default)",
		Example: strings.Trim(`
  partiful-pp-cli events list
  partiful-pp-cli events list --hosting --all
  partiful-pp-cli events list --past --limit 5 --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return dryRunNote(cmd, flags, "would list events")
			}
			pc, err := newPartiful(flags)
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			wantUpcoming := !past || all
			wantPast := past || all
			events, err := pc.ListEvents(ctx, wantUpcoming, wantPast)
			if err != nil {
				return partifulErr(err)
			}
			out := events[:0:0]
			for _, e := range events {
				if hosting && e.Role != "host" {
					continue
				}
				out = append(out, e)
				if limit > 0 && len(out) >= limit {
					break
				}
			}
			return printLive(cmd, flags, out)
		},
	}
	cmd.Flags().BoolVar(&past, "past", false, "Show past events instead of upcoming ones")
	cmd.Flags().BoolVar(&all, "all", false, "Show upcoming and past events")
	cmd.Flags().BoolVar(&hosting, "hosting", false, "Only events you host or cohost")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum number of events to show (0 = all)")
	return cmd
}

func newEventsGetCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "get <event-id-or-url>",
		Short:       "Show one event's full details",
		Example:     "  partiful-pp-cli events get https://partiful.com/e/hqWkFzikB1frVdihGKTE",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:happy-args": "<event-id-or-url>=hqWkFzikB1frVdihGKTE"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return dryRunNote(cmd, flags, "would fetch the event")
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
			ev, err := pc.GetEvent(ctx, id)
			if err != nil {
				return partifulErr(err)
			}
			return printLive(cmd, flags, ev)
		},
	}
	return cmd
}

// eventInput holds the shared create/update flags.
type eventInput struct {
	title, start, end, timezone, description, descriptionFile, location, image string
	public, private                                                            bool
}

func (in *eventInput) register(cmd *cobra.Command) {
	cmd.Flags().StringVar(&in.title, "title", "", "Event title")
	cmd.Flags().StringVar(&in.start, "start", "", "Start time: \"2026-12-12 09:00\" (in --timezone) or RFC3339")
	cmd.Flags().StringVar(&in.end, "end", "", "End time: \"2026-12-12 12:00\" or RFC3339; \"none\" clears it")
	cmd.Flags().StringVar(&in.timezone, "timezone", "", "IANA timezone, e.g. America/New_York")
	cmd.Flags().StringVar(&in.description, "description", "", "Event description (plain text, newlines allowed)")
	cmd.Flags().StringVar(&in.descriptionFile, "description-file", "", "Read the description from a file")
	cmd.Flags().StringVar(&in.location, "location", "", "Location as you would type it, e.g. \"Pentridge Manor, 1034 S 53rd St, Philadelphia, PA\"")
	cmd.Flags().StringVar(&in.image, "image", "", "Path to a PNG, JPEG or GIF cover image to upload")
	cmd.Flags().BoolVar(&in.public, "public", false, "Make the event public (listed and shareable)")
	cmd.Flags().BoolVar(&in.private, "private", false, "Make the event private")
}

// fields converts flags into event-document field values. Image upload is
// handled separately because it has side effects.
func (in *eventInput) fields(tzDefault string) (map[string]any, error) {
	f := map[string]any{}
	tz := in.timezone
	if tz == "" {
		tz = tzDefault
	}
	if in.timezone != "" {
		if _, err := time.LoadLocation(in.timezone); err != nil {
			return nil, usageErr(fmt.Errorf("unknown --timezone %q", in.timezone))
		}
		f["timezone"] = in.timezone
	}
	if in.title != "" {
		f["title"] = in.title
	}
	if in.start != "" {
		t, err := partiful.ParseTime(in.start, tz)
		if err != nil {
			return nil, usageErr(err)
		}
		f["startDate"] = t
	}
	switch strings.ToLower(in.end) {
	case "":
	case "none":
		f["endDate"] = nil
	default:
		t, err := partiful.ParseTime(in.end, tz)
		if err != nil {
			return nil, usageErr(err)
		}
		f["endDate"] = t
	}
	if in.descriptionFile != "" {
		b, err := os.ReadFile(in.descriptionFile)
		if err != nil {
			return nil, usageErr(err)
		}
		f["description"] = strings.TrimRight(string(b), "\n")
	} else if in.description != "" {
		f["description"] = in.description
	}
	if in.public && in.private {
		return nil, usageErr(fmt.Errorf("--public and --private are mutually exclusive"))
	}
	if in.public {
		f["isPublic"] = true
	}
	if in.private {
		f["isPublic"] = false
	}
	if s, ok := f["startDate"].(time.Time); ok {
		if e, ok := f["endDate"].(time.Time); ok && !e.After(s) {
			return nil, usageErr(fmt.Errorf("--end must be after --start"))
		}
	}
	return f, nil
}

func displayValue(v any) any {
	switch x := v.(type) {
	case time.Time:
		return x.UTC().Format(time.RFC3339)
	case string:
		if len(x) > 160 {
			return x[:157] + "..."
		}
	case map[string]any:
		if u, ok := x["url"].(string); ok {
			return "image " + u
		}
	}
	return v
}

type fieldChange struct {
	Field string `json:"field"`
	From  any    `json:"from"`
	To    any    `json:"to"`
}

func newEventsCreateCmd(flags *rootFlags) *cobra.Command {
	var in eventInput
	var copyFrom string
	var apply bool
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a new event (preview by default; --apply to create)",
		Annotations: map[string]string{"pp:method": "POST"},
		Long: `Creates an event on your account. Without --apply it only prints the event that
would be created. --copy-from reuses an earlier event's image, theme, description and
settings, which is handy for a recurring series; flags override copied values.`,
		Example: strings.Trim(`
  partiful-pp-cli events create --copy-from hqWkFzikB1frVdihGKTE --title "Breakfast and Building" --start "2026-12-12 09:00" --end "2026-12-12 12:00"
  partiful-pp-cli events create --title "Vibe Code Philly 07" --start "2026-12-05 18:00" --location "Philadelphia, PA" --image ./cover.png --apply`, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return dryRunNote(cmd, flags, "would preview a new event")
			}
			if in.title == "" || in.start == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--title and --start are required"))
			}
			if in.image != "" {
				if _, err := partiful.ReadImage(in.image); err != nil {
					return usageErr(err)
				}
			}
			pc, err := newPartiful(flags)
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			tz := in.timezone
			event := partiful.NewEventDefaults("America/New_York")
			if copyFrom != "" {
				srcID, err := partiful.ParseEventID(copyFrom)
				if err != nil {
					return usageErr(err)
				}
				doc, err := pc.EventDocument(ctx, srcID)
				if err != nil {
					return partifulErr(err)
				}
				for _, k := range partiful.TemplateFields {
					if v, ok := doc.Fields[k]; ok && v != nil {
						event[k] = v
					}
				}
			}
			if tz == "" {
				tz, _ = event["timezone"].(string)
			}
			fields, err := in.fields(tz)
			if err != nil {
				return err
			}
			for k, v := range fields {
				event[k] = v
			}
			if _, hasEnd := fields["endDate"]; !hasEnd {
				delete(event, "endDate")
			}

			preview := map[string]any{"action": "create", "event": displayEvent(event), "applied": false}
			if in.location != "" {
				preview["location"] = in.location
			}
			if in.image != "" {
				preview["uploadImage"] = in.image
			}
			if !apply || cliutil.IsVerifyEnv() {
				preview["note"] = applyHint
				return flags.printJSON(cmd, preview)
			}

			if in.image != "" {
				img, _ := partiful.ReadImage(in.image)
				obj, err := pc.UploadImage(ctx, img)
				if err != nil {
					return partifulErr(err)
				}
				event["image"] = obj
			}
			id, err := pc.CreateEvent(ctx, jsonSafe(event))
			if err != nil {
				return partifulErr(err)
			}
			result := map[string]any{"action": "create", "applied": true, "eventId": id, "url": partiful.EventURL(id)}
			if in.location != "" {
				if err := pc.SetLocation(ctx, id, in.location); err != nil {
					result["locationError"] = err.Error()
				}
			}
			if doc, err := pc.EventDocument(ctx, id); err == nil {
				result["status"] = doc.Fields["status"]
			}
			return flags.printJSON(cmd, result)
		},
	}
	in.register(cmd)
	cmd.Flags().StringVar(&copyFrom, "copy-from", "", "Event id or URL to copy image, theme, description and settings from")
	cmd.Flags().BoolVar(&apply, "apply", false, "Actually create the event (default is a preview)")
	return cmd
}

// jsonSafe converts time.Time values to ISO strings for the callable payload.
func jsonSafe(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		switch x := v.(type) {
		case time.Time:
			out[k] = x.UTC().Format("2006-01-02T15:04:05.000Z")
		case map[string]any:
			out[k] = jsonSafe(x)
		default:
			out[k] = v
		}
	}
	return out
}

func displayEvent(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = displayValue(v)
	}
	return out
}

func newEventsUpdateCmd(flags *rootFlags) *cobra.Command {
	var in eventInput
	var apply bool
	cmd := &cobra.Command{
		Use:   "update <event-id-or-url>",
		Short: "Edit an event's details (preview by default; --apply to save)",
		Long: `Shows a field-by-field diff of what would change. Nothing is saved unless you pass
--apply. Only the fields you pass are touched.`,
		Example: strings.Trim(`
  partiful-pp-cli events update hqWkFzikB1frVdihGKTE --title "Breakfast and Building 02"
  partiful-pp-cli events update hqWkFzikB1frVdihGKTE --start "2026-11-14 09:30" --location "1034 S 53rd St, Philadelphia, PA" --apply`, "\n"),
		Annotations: map[string]string{"pp:method": "PATCH", "pp:happy-args": "<event-id-or-url>=hqWkFzikB1frVdihGKTE;--title=Example"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return dryRunNote(cmd, flags, "would preview event changes")
			}
			if len(args) != 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("an event id or URL is required"))
			}
			id, err := partiful.ParseEventID(args[0])
			if err != nil {
				return usageErr(err)
			}
			if in.image != "" {
				if _, err := partiful.ReadImage(in.image); err != nil {
					return usageErr(err)
				}
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
			tz, _ := doc.Fields["timezone"].(string)
			if tz == "" {
				tz = "America/New_York"
			}
			fields, err := in.fields(tz)
			if err != nil {
				return err
			}
			changes := []fieldChange{}
			for _, k := range partiful.EditableFields {
				v, ok := fields[k]
				if !ok {
					continue
				}
				cur := doc.Fields[k]
				if sameValue(cur, v) {
					delete(fields, k)
					continue
				}
				changes = append(changes, fieldChange{Field: k, From: displayValue(cur), To: displayValue(v)})
			}
			if in.location != "" {
				changes = append(changes, fieldChange{Field: "location", From: currentLocation(doc.Fields), To: in.location})
			}
			if in.image != "" {
				changes = append(changes, fieldChange{Field: "image", From: displayValue(doc.Fields["image"]), To: "upload " + in.image})
			}
			title, _ := doc.Fields["title"].(string)
			plan := map[string]any{"action": "update", "eventId": id, "title": title, "url": partiful.EventURL(id), "changes": changes, "applied": false}
			if len(changes) == 0 {
				plan["note"] = "No changes: every value you passed already matches the event."
				return flags.printJSON(cmd, plan)
			}
			if !apply || cliutil.IsVerifyEnv() {
				plan["note"] = applyHint
				return flags.printJSON(cmd, plan)
			}
			if in.image != "" {
				img, _ := partiful.ReadImage(in.image)
				obj, err := pc.UploadImage(ctx, img)
				if err != nil {
					return partifulErr(err)
				}
				fields["image"] = obj
			}
			if len(fields) > 0 {
				if _, err := pc.UpdateEvent(ctx, id, fields); err != nil {
					return partifulErr(err)
				}
			}
			if in.location != "" {
				if err := pc.SetLocation(ctx, id, in.location); err != nil {
					return partifulErr(err)
				}
			}
			plan["applied"] = true
			return flags.printJSON(cmd, plan)
		},
	}
	in.register(cmd)
	cmd.Flags().BoolVar(&apply, "apply", false, "Actually save the changes (default is a preview)")
	return cmd
}

func currentLocation(f map[string]any) any {
	if li, ok := f["locationInfo"].(map[string]any); ok {
		if v, ok := li["value"]; ok {
			return v
		}
		if lines, ok := li["displayAddressLines"].([]any); ok {
			parts := make([]string, 0, len(lines))
			for _, l := range lines {
				parts = append(parts, fmt.Sprint(l))
			}
			return strings.Join(parts, ", ")
		}
	}
	return f["location"]
}

// sameValue compares a decoded Firestore value with a new flag value.
func sameValue(cur, next any) bool {
	if t, ok := next.(time.Time); ok {
		s, ok := cur.(string)
		if !ok {
			return false
		}
		ct, err := time.Parse(time.RFC3339Nano, s)
		return err == nil && ct.Equal(t)
	}
	return reflect.DeepEqual(cur, next)
}

func newEventsPublishCmd(flags *rootFlags) *cobra.Command {
	var apply bool
	cmd := &cobra.Command{
		Use:         "publish <event-id-or-url>",
		Short:       "Publish a draft event (preview by default; --apply to publish)",
		Example:     "  partiful-pp-cli events publish hqWkFzikB1frVdihGKTE --apply",
		Annotations: map[string]string{"pp:method": "POST", "pp:happy-args": "<event-id-or-url>=hqWkFzikB1frVdihGKTE"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return dryRunNote(cmd, flags, "would preview publishing the event")
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
			plan := map[string]any{"action": "publish", "eventId": id, "title": doc.Fields["title"], "currentStatus": doc.Fields["status"], "applied": false}
			if !apply || cliutil.IsVerifyEnv() {
				plan["note"] = applyHint
				return flags.printJSON(cmd, plan)
			}
			if err := pc.PublishEvent(ctx, id); err != nil {
				return partifulErr(err)
			}
			plan["applied"] = true
			return flags.printJSON(cmd, plan)
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "Actually publish (default is a preview)")
	return cmd
}
