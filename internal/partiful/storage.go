// Copyright 2026 sirakinb and contributors. Licensed under Apache-2.0. See LICENSE.

package partiful

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"  // register decoders for DecodeConfig
	_ "image/jpeg" // register decoders for DecodeConfig
	_ "image/png"  // register decoders for DecodeConfig
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ImageFile describes a local image prepared for upload.
type ImageFile struct {
	Path        string
	Name        string
	ContentType string
	Width       int
	Height      int
	Size        int64
	data        []byte
}

// ReadImage loads and inspects a local PNG, JPEG or GIF.
func ReadImage(path string) (*ImageFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%s is not a PNG, JPEG or GIF image: %w", path, err)
	}
	ct := map[string]string{"png": "image/png", "jpeg": "image/jpeg", "gif": "image/gif"}[format]
	return &ImageFile{Path: path, Name: filepath.Base(path), ContentType: ct, Width: cfg.Width, Height: cfg.Height, Size: int64(len(data)), data: data}, nil
}

const idAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz_-"

func randomID(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = idAlphabet[int(b[i])%len(idAlphabet)]
	}
	return string(b)
}

// UploadImage stores the image where the Partiful web app stores user uploads
// (external/user/<uid>/<random>) and returns the event "image" object the web
// app writes for an uploaded cover image.
func (c *Client) UploadImage(ctx context.Context, img *ImageFile) (map[string]any, error) {
	uid, err := c.UserID(ctx)
	if err != nil {
		return nil, err
	}
	objPath := "external/user/" + uid + "/" + randomID(21)
	u := c.StorageBase + "?" + url.Values{"name": {objPath}}.Encode()
	raw, err := c.do(ctx, "image upload", http.MethodPost, u, bytes.NewReader(img.data), img.ContentType, "Firebase")
	if err != nil {
		return nil, err
	}
	var meta struct {
		Name           string `json:"name"`
		DownloadTokens string `json:"downloadTokens"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, err
	}
	dl := c.StorageBase + "/" + url.PathEscape(objPath) + "?alt=media"
	if tok := strings.Split(meta.DownloadTokens, ",")[0]; tok != "" {
		dl += "&token=" + tok
	}
	now := time.Now().UTC()
	upload := map[string]any{
		"type":          "image",
		"fileCreatedAt": now,
		"uploadedAt":    now,
		"path":          objPath,
		"name":          img.Name,
		"width":         img.Width,
		"height":        img.Height,
		"contentType":   img.ContentType,
		"storageUri":    "gs://" + bucket + "/" + objPath,
		"url":           dl,
		"size":          img.Size,
	}
	return map[string]any{
		"source":      "upload",
		"type":        "image",
		"upload":      upload,
		"url":         dl,
		"contentType": img.ContentType,
		"name":        img.Name,
		"width":       img.Width,
		"height":      img.Height,
	}, nil
}
