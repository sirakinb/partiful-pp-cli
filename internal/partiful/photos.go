// Copyright 2026 sirakinb and contributors. Licensed under Apache-2.0. See LICENSE.

package partiful

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"path/filepath"
)

// Photo album uploads work the way the partiful.com photo album does:
//
//  1. Each file is POSTed as multipart form data to
//     https://api.partiful.com/uploadPhoto?uploadType=event_photo&eventId=<id>
//     (with the user id in an x-user-id header). The response describes the
//     stored file ("uploadData").
//  2. One createComment call attaches all uploaded files to a new post in the
//     event's activity feed. Photos only appear in the album once they are
//     attached to a post, so this post is visible to guests.

// AlbumPhoto is one item from an event's photo album.
type AlbumPhoto struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	URL       string `json:"url"`
	CreatedAt string `json:"createdAt,omitempty"`
	UserID    string `json:"userId,omitempty"`
	CommentID string `json:"commentId,omitempty"`
}

// ListAlbum returns the photos and videos in an event's photo album.
func (c *Client) ListAlbum(ctx context.Context, eventID string) ([]AlbumPhoto, error) {
	raw, err := c.Call(ctx, "getEventMedia", map[string]any{"params": map[string]any{"eventId": eventID}})
	if err != nil {
		return nil, err
	}
	var d struct {
		Media []struct {
			ID        string `json:"id"`
			Type      string `json:"type"`
			CreatedAt string `json:"createdAt"`
			User      struct {
				ID string `json:"id"`
			} `json:"user"`
			Comment struct {
				ID string `json:"id"`
			} `json:"comment"`
			Image struct {
				URL string `json:"url"`
			} `json:"image"`
			Video struct {
				URL string `json:"url"`
			} `json:"video"`
		} `json:"media"`
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, err
	}
	out := make([]AlbumPhoto, 0, len(d.Media))
	for _, m := range d.Media {
		u := m.Image.URL
		if u == "" {
			u = m.Video.URL
		}
		out = append(out, AlbumPhoto{ID: m.ID, Type: m.Type, URL: u, CreatedAt: m.CreatedAt, UserID: m.User.ID, CommentID: m.Comment.ID})
	}
	return out, nil
}

// UploadEventPhoto uploads one image for an event and returns the upload
// record the web app passes on to createComment.
func (c *Client) UploadEventPhoto(ctx context.Context, eventID string, img *ImageFile) (map[string]any, error) {
	uid, err := c.UserID(ctx)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, filepath.Base(img.Path)))
	h.Set("Content-Type", img.ContentType)
	part, err := w.CreatePart(h)
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(img.data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	q := url.Values{"uploadType": {"event_photo"}, "eventId": {eventID}}
	raw, err := c.doWithHeaders(ctx, "uploadPhoto", http.MethodPost, c.APIBase+"/uploadPhoto?"+q.Encode(),
		&buf, w.FormDataContentType(), "Bearer", map[string]string{"x-user-id": uid})
	if err != nil {
		return nil, err
	}
	var resp map[string]any
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("uploadPhoto: unexpected response: %w", err)
	}
	if r, ok := resp["result"].(map[string]any); ok {
		resp = r
	}
	data, ok := resp["uploadData"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("uploadPhoto: response has no uploadData")
	}
	return data, nil
}

// MediaFromUpload converts an upload record into the media item createComment expects.
func MediaFromUpload(upload map[string]any) map[string]any {
	kind := "image"
	if t, _ := upload["type"].(string); t == "video" {
		kind = "video"
	}
	return map[string]any{"source": "upload", "type": kind, "url": upload["url"], "upload": upload}
}

// PostPhotos publishes one activity-feed post with the given media, which is
// what adds them to the event's photo album. message may be empty.
func (c *Client) PostPhotos(ctx context.Context, eventID, timezone, message string, media []map[string]any) (json.RawMessage, error) {
	params := map[string]any{"eventId": eventID, "timezone": timezone, "media": media}
	if message != "" {
		params["message"] = message
	}
	return c.Call(ctx, "createComment", map[string]any{"params": params})
}
