// Copyright 2026 sirakinb and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sirakinb/partiful-pp-cli/internal/cliutil"
	"github.com/sirakinb/partiful-pp-cli/internal/partiful"
)

func newPhotosCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "photos",
		Short: "See and add photos in an event's photo album",
		RunE:  parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newPhotosListCmd(flags), newPhotosAddCmd(flags))
	return cmd
}

func newPhotosListCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:         "list <event-id-or-url>",
		Short:       "List the photos and videos in an event's photo album",
		Example:     "  partiful-pp-cli photos list hqWkFzikB1frVdihGKTE",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:happy-args": "<event-id-or-url>=hqWkFzikB1frVdihGKTE"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return dryRunNote(cmd, flags, "would list album photos")
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
			photos, err := pc.ListAlbum(ctx, id)
			if err != nil {
				return partifulErr(err)
			}
			return printLive(cmd, flags, photos)
		},
	}
}

func newPhotosAddCmd(flags *rootFlags) *cobra.Command {
	var message string
	var apply bool
	cmd := &cobra.Command{
		Use:   "add <event-id-or-url> <image>...",
		Short: "Add photos to an event's photo album (preview by default; --apply to upload)",
		Long: `Uploads PNG, JPEG or GIF files to an event's photo album, the same way the
"Add photos" button on partiful.com does. All files go up first; then one post with
every photo is added to the event's activity feed, which is what puts them in the
album. That post is visible to guests and Partiful may notify them about it.

Without --apply this only prints what would be uploaded.`,
		Example: strings.Trim(`
  partiful-pp-cli photos add hqWkFzikB1frVdihGKTE ./photos/*.jpg
  partiful-pp-cli photos add hqWkFzikB1frVdihGKTE a.jpg b.jpg --message "Some moments from past events" --apply`, "\n"),
		Annotations: map[string]string{"pp:method": "POST", "pp:happy-args": "<event-id-or-url>=hqWkFzikB1frVdihGKTE"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return dryRunNote(cmd, flags, "would preview a photo album upload")
			}
			if len(args) < 2 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("an event id or URL and at least one image file are required"))
			}
			id, err := partiful.ParseEventID(args[0])
			if err != nil {
				return usageErr(err)
			}
			images := make([]*partiful.ImageFile, 0, len(args)-1)
			var total int64
			files := make([]map[string]any, 0, len(args)-1)
			for _, p := range args[1:] {
				img, err := partiful.ReadImage(p)
				if err != nil {
					return usageErr(err)
				}
				images = append(images, img)
				total += img.Size
				files = append(files, map[string]any{"file": img.Name, "width": img.Width, "height": img.Height,
					"sizeMB": float64(img.Size/10000) / 100})
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
			plan := map[string]any{
				"action":  "photos add",
				"eventId": id,
				"title":   doc.Fields["title"],
				"url":     partiful.EventURL(id),
				"photos":  files,
				"count":   len(images),
				"totalMB": float64(total/10000) / 100,
				"post":    "One activity-feed post with these photos will be added (visible to guests; Partiful may notify them).",
				"applied": false,
			}
			if message != "" {
				plan["message"] = message
			}
			if !apply || cliutil.IsVerifyEnv() {
				plan["note"] = applyHint
				return flags.printJSON(cmd, plan)
			}
			media := make([]map[string]any, 0, len(images))
			for i, img := range images {
				up, err := pc.UploadEventPhoto(ctx, id, img)
				if err != nil {
					return partifulErr(fmt.Errorf("uploading %s (%d of %d): %w; no album post was created", img.Name, i+1, len(images), err))
				}
				media = append(media, partiful.MediaFromUpload(up))
				if !flags.quiet && !flags.asJSON {
					fmt.Fprintf(cmd.ErrOrStderr(), "uploaded %d/%d %s\n", i+1, len(images), img.Name)
				}
			}
			if _, err := pc.PostPhotos(ctx, id, tz, message, media); err != nil {
				return partifulErr(fmt.Errorf("photos uploaded but the album post failed: %w", err))
			}
			plan["applied"] = true
			if album, err := pc.ListAlbum(ctx, id); err == nil {
				plan["albumCount"] = len(album)
			}
			return flags.printJSON(cmd, plan)
		},
	}
	cmd.Flags().StringVar(&message, "message", "", "Optional caption for the activity-feed post")
	cmd.Flags().BoolVar(&apply, "apply", false, "Actually upload and post the photos (default is a preview)")
	return cmd
}
