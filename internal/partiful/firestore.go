// Copyright 2026 sirakinb and contributors. Licensed under Apache-2.0. See LICENSE.

package partiful

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"
)

// Ref is a Firestore document reference, written as a referenceValue.
// Use DocRef to build one from a relative path like "users/<uid>".
type Ref string

// DocRef returns the fully qualified reference for a relative document path.
func DocRef(relPath string) Ref {
	return Ref("projects/" + projectID + "/databases/(default)/documents/" + relPath)
}

// Document is a decoded Firestore document.
type Document struct {
	Name       string         `json:"name"`
	ID         string         `json:"id"`
	Fields     map[string]any `json:"fields"`
	CreateTime string         `json:"createTime,omitempty"`
	UpdateTime string         `json:"updateTime,omitempty"`
}

type rawDocument struct {
	Name       string                     `json:"name"`
	Fields     map[string]json.RawMessage `json:"fields"`
	CreateTime string                     `json:"createTime"`
	UpdateTime string                     `json:"updateTime"`
}

func (r rawDocument) decode() (*Document, error) {
	fields := make(map[string]any, len(r.Fields))
	for k, v := range r.Fields {
		dv, err := decodeValue(v)
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", k, err)
		}
		fields[k] = dv
	}
	id := r.Name
	for i := len(id) - 1; i >= 0; i-- {
		if id[i] == '/' {
			id = id[i+1:]
			break
		}
	}
	return &Document{Name: r.Name, ID: id, Fields: fields, CreateTime: r.CreateTime, UpdateTime: r.UpdateTime}, nil
}

// GetDocument reads one document by relative path, e.g. "events/<id>".
func (c *Client) GetDocument(ctx context.Context, relPath string) (*Document, error) {
	raw, err := c.do(ctx, "firestore get "+relPath, http.MethodGet, c.FirestoreBase+"/"+relPath, nil, "", "Bearer")
	if err != nil {
		return nil, err
	}
	var rd rawDocument
	if err := json.Unmarshal(raw, &rd); err != nil {
		return nil, err
	}
	return rd.decode()
}

// ListDocuments pages through a collection (e.g. "events/<id>/guests").
// max <= 0 means no limit.
func (c *Client) ListDocuments(ctx context.Context, relPath string, max int) ([]*Document, error) {
	var out []*Document
	pageToken := ""
	for {
		q := url.Values{"pageSize": {"300"}}
		if pageToken != "" {
			q.Set("pageToken", pageToken)
		}
		raw, err := c.do(ctx, "firestore list "+relPath, http.MethodGet, c.FirestoreBase+"/"+relPath+"?"+q.Encode(), nil, "", "Bearer")
		if err != nil {
			return nil, err
		}
		var page struct {
			Documents     []rawDocument `json:"documents"`
			NextPageToken string        `json:"nextPageToken"`
		}
		if err := json.Unmarshal(raw, &page); err != nil {
			return nil, err
		}
		for _, rd := range page.Documents {
			d, err := rd.decode()
			if err != nil {
				return nil, err
			}
			out = append(out, d)
			if max > 0 && len(out) >= max {
				return out, nil
			}
		}
		if page.NextPageToken == "" {
			return out, nil
		}
		pageToken = page.NextPageToken
	}
}

// PatchDocument updates only the given top-level fields (an update mask), and
// requires the document to already exist so a typo cannot create a new one.
func (c *Client) PatchDocument(ctx context.Context, relPath string, fields map[string]any) (*Document, error) {
	keys := make([]string, 0, len(fields))
	enc := make(map[string]any, len(fields))
	for k, v := range fields {
		keys = append(keys, k)
		enc[k] = EncodeValue(v)
	}
	sort.Strings(keys)
	q := url.Values{"currentDocument.exists": {"true"}}
	for _, k := range keys {
		q.Add("updateMask.fieldPaths", "`"+k+"`")
	}
	body, err := json.Marshal(map[string]any{"fields": enc})
	if err != nil {
		return nil, err
	}
	raw, err := c.do(ctx, "firestore update "+relPath, http.MethodPatch, c.FirestoreBase+"/"+relPath+"?"+q.Encode(), bytes.NewReader(body), "application/json", "Bearer")
	if err != nil {
		return nil, err
	}
	var rd rawDocument
	if err := json.Unmarshal(raw, &rd); err != nil {
		return nil, err
	}
	return rd.decode()
}

// EncodeValue converts a Go value into a Firestore REST Value.
func EncodeValue(v any) map[string]any {
	switch x := v.(type) {
	case nil:
		return map[string]any{"nullValue": nil}
	case bool:
		return map[string]any{"booleanValue": x}
	case string:
		return map[string]any{"stringValue": x}
	case Ref:
		return map[string]any{"referenceValue": string(x)}
	case int:
		return map[string]any{"integerValue": strconv.Itoa(x)}
	case int64:
		return map[string]any{"integerValue": strconv.FormatInt(x, 10)}
	case float64:
		if x == float64(int64(x)) {
			return map[string]any{"integerValue": strconv.FormatInt(int64(x), 10)}
		}
		return map[string]any{"doubleValue": x}
	case time.Time:
		return map[string]any{"timestampValue": x.UTC().Format(time.RFC3339Nano)}
	case []any:
		vals := make([]any, 0, len(x))
		for _, e := range x {
			vals = append(vals, EncodeValue(e))
		}
		return map[string]any{"arrayValue": map[string]any{"values": vals}}
	case map[string]any:
		f := make(map[string]any, len(x))
		for k, e := range x {
			f[k] = EncodeValue(e)
		}
		return map[string]any{"mapValue": map[string]any{"fields": f}}
	default:
		// Round-trip anything else (structs, typed slices) through JSON.
		b, _ := json.Marshal(x)
		var generic any
		_ = json.Unmarshal(b, &generic)
		return EncodeValue(generic)
	}
}

func decodeValue(raw json.RawMessage) (any, error) {
	var v map[string]json.RawMessage
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	for kind, inner := range v {
		switch kind {
		case "nullValue":
			return nil, nil
		case "booleanValue":
			var b bool
			err := json.Unmarshal(inner, &b)
			return b, err
		case "stringValue", "referenceValue", "timestampValue", "bytesValue":
			var s string
			err := json.Unmarshal(inner, &s)
			return s, err
		case "integerValue":
			var s string
			if err := json.Unmarshal(inner, &s); err != nil {
				return nil, err
			}
			n, err := strconv.ParseInt(s, 10, 64)
			return n, err
		case "doubleValue":
			var f float64
			err := json.Unmarshal(inner, &f)
			return f, err
		case "geoPointValue":
			var g map[string]float64
			err := json.Unmarshal(inner, &g)
			return g, err
		case "arrayValue":
			var a struct {
				Values []json.RawMessage `json:"values"`
			}
			if err := json.Unmarshal(inner, &a); err != nil {
				return nil, err
			}
			out := make([]any, 0, len(a.Values))
			for _, e := range a.Values {
				d, err := decodeValue(e)
				if err != nil {
					return nil, err
				}
				out = append(out, d)
			}
			return out, nil
		case "mapValue":
			var m struct {
				Fields map[string]json.RawMessage `json:"fields"`
			}
			if err := json.Unmarshal(inner, &m); err != nil {
				return nil, err
			}
			out := make(map[string]any, len(m.Fields))
			for k, e := range m.Fields {
				d, err := decodeValue(e)
				if err != nil {
					return nil, err
				}
				out[k] = d
			}
			return out, nil
		}
	}
	return nil, nil
}
