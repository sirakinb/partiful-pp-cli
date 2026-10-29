// Copyright 2026 sirakinb and contributors. Licensed under Apache-2.0. See LICENSE.

package partiful

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"strings"
	"testing"
)

func tinyPNG(t *testing.T) *ImageFile {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 4, 3))); err != nil {
		t.Fatal(err)
	}
	return &ImageFile{Path: "/tmp/a.png", Name: "a.png", ContentType: "image/png", Width: 4, Height: 3, Size: int64(buf.Len()), data: buf.Bytes()}
}

func TestUploadEventPhotoRequestShape(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/uploadPhoto" || r.Method != http.MethodPost {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("uploadType") != "event_photo" || q.Get("eventId") != "e1" {
			t.Errorf("query = %v", q)
		}
		if r.Header.Get("x-user-id") != "uid1" || r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("headers: user=%q auth=%q", r.Header.Get("x-user-id"), r.Header.Get("Authorization"))
		}
		f, fh, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("no file part: %v", err)
		}
		b, _ := io.ReadAll(f)
		if fh.Filename != "a.png" || len(b) == 0 {
			t.Errorf("file part %q, %d bytes", fh.Filename, len(b))
		}
		io.WriteString(w, `{"uploadData":{"type":"image","url":"https://x/y.png","path":"p/y"}}`)
	})
	up, err := c.UploadEventPhoto(context.Background(), "e1", tinyPNG(t))
	if err != nil || up["url"] != "https://x/y.png" {
		t.Fatalf("UploadEventPhoto = %v, %v", up, err)
	}
	m := MediaFromUpload(up)
	if m["source"] != "upload" || m["type"] != "image" || m["url"] != "https://x/y.png" {
		t.Errorf("media = %v", m)
	}
}

func TestPostPhotosEnvelope(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/createComment" {
			t.Errorf("path %s", r.URL.Path)
		}
		var body struct {
			Data struct {
				Params map[string]any `json:"params"`
			} `json:"data"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		p := body.Data.Params
		if p["eventId"] != "e1" || p["timezone"] != "America/New_York" || len(p["media"].([]any)) != 2 {
			t.Errorf("params = %v", p)
		}
		if _, ok := p["message"]; ok {
			t.Error("empty message should be omitted")
		}
		io.WriteString(w, `{"result":{"data":null}}`)
	})
	media := []map[string]any{MediaFromUpload(map[string]any{"url": "u1"}), MediaFromUpload(map[string]any{"url": "u2"})}
	if _, err := c.PostPhotos(context.Background(), "e1", "America/New_York", "", media); err != nil {
		t.Fatal(err)
	}
}

func TestListAlbum(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"result":{"data":{"media":[{"id":"m1","type":"image","image":{"url":"https://i/1"},"user":{"id":"u"},"comment":{"id":"c1"},"createdAt":"2026-10-17T18:00:00Z"}]}}}`)
	})
	got, err := c.ListAlbum(context.Background(), "e1")
	if err != nil || len(got) != 1 || got[0].URL != "https://i/1" || got[0].CommentID != "c1" {
		t.Fatalf("ListAlbum = %+v, %v", got, err)
	}
	if !strings.HasPrefix(got[0].CreatedAt, "2026") {
		t.Errorf("createdAt = %q", got[0].CreatedAt)
	}
}
