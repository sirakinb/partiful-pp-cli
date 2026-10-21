// Copyright 2026 sirakinb and contributors. Licensed under Apache-2.0. See LICENSE.

package partiful

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseEventID(t *testing.T) {
	tests := []struct {
		in, want string
		wantErr  bool
	}{
		{"hqWkFzikB1frVdihGKTE", "hqWkFzikB1frVdihGKTE", false},
		{"https://partiful.com/e/hqWkFzikB1frVdihGKTE", "hqWkFzikB1frVdihGKTE", false},
		{"https://partiful.com/e/hqWkFzikB1frVdihGKTE?c=abc", "hqWkFzikB1frVdihGKTE", false},
		{"  hqWkFzikB1frVdihGKTE  ", "hqWkFzikB1frVdihGKTE", false},
		{"not-an-id", "", true},
		{"https://partiful.com/u/someone", "", true},
	}
	for _, tt := range tests {
		got, err := ParseEventID(tt.in)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("ParseEventID(%q) = %q, %v; want %q, err=%v", tt.in, got, err, tt.want, tt.wantErr)
		}
	}
}

func TestParseTime(t *testing.T) {
	got, err := ParseTime("2026-12-12 09:00", "America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	if want := "2026-12-12T14:00:00Z"; got.UTC().Format(time.RFC3339) != want {
		t.Errorf("got %s want %s", got.UTC().Format(time.RFC3339), want)
	}
	if _, err := ParseTime("2026-12-12T09:00:00-05:00", "Nowhere/Invalid"); err != nil {
		t.Errorf("RFC3339 should not need a valid tz: %v", err)
	}
	if _, err := ParseTime("next saturday", "America/New_York"); err == nil {
		t.Error("expected error for free text")
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	ts := time.Date(2026, 11, 14, 14, 0, 0, 0, time.UTC)
	in := map[string]any{"title": "Breakfast", "isPublic": true, "count": 3, "endDate": nil,
		"startDate": ts, "updatedBy": DocRef("users/abc"),
		"displaySettings": map[string]any{"theme": "oxblood"}, "tags": []any{"a", "b"}}
	enc := EncodeValue(in)
	raw, _ := json.Marshal(enc)
	dec, err := decodeValue(raw)
	if err != nil {
		t.Fatal(err)
	}
	m := dec.(map[string]any)
	if m["title"] != "Breakfast" || m["isPublic"] != true || m["count"] != int64(3) || m["endDate"] != nil {
		t.Errorf("scalar round trip failed: %#v", m)
	}
	if m["startDate"] != "2026-11-14T14:00:00Z" {
		t.Errorf("timestamp = %v", m["startDate"])
	}
	if m["updatedBy"] != "projects/getpartiful/databases/(default)/documents/users/abc" {
		t.Errorf("reference = %v", m["updatedBy"])
	}
	if m["displaySettings"].(map[string]any)["theme"] != "oxblood" || len(m["tags"].([]any)) != 2 {
		t.Errorf("nested round trip failed: %#v", m)
	}
}

func TestSummarize(t *testing.T) {
	s := Summarize("e1", []Guest{
		{Status: "GOING", Count: 2},
		{Status: "GOING", PlusOnes: 1},
		{Status: "MAYBE", Count: 1},
		{Status: "DECLINED"},
	})
	if s.Total != 4 || s.Headcount != 4 || s.ByStatus["GOING"] != 2 || s.PeopleBy["MAYBE"] != 1 || s.PeopleBy["DECLINED"] != 1 {
		t.Errorf("unexpected summary: %+v", s)
	}
	if empty := Summarize("e2", nil); empty.Total != 0 || empty.Headcount != 0 {
		t.Errorf("empty summary should be zero: %+v", empty)
	}
}

// testClient returns a client whose token endpoint, callables and Firestore
// all point at one httptest server, with a pre-minted ID token.
func testClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	t.Setenv(SessionPathEnv, filepath.Join(t.TempDir(), "session.json"))
	s := &Session{RefreshToken: "r", UserID: "uid1", IDToken: "tok", ExpiresAt: time.Now().Add(time.Hour)}
	c := NewWithSession(s, 5*time.Second)
	c.APIBase, c.FirestoreBase, c.StorageBase, c.TokenURL = srv.URL+"/api", srv.URL+"/fs", srv.URL+"/st", srv.URL+"/token"
	return c
}

func TestCallEnvelope(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/getEventsByIds" || r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("bad request %s auth=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		var body struct {
			Data map[string]any `json:"data"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Data["userId"] != "uid1" || body.Data["params"].(map[string]any)["eventIds"] == nil {
			t.Errorf("bad envelope: %#v", body.Data)
		}
		io.WriteString(w, `{"result":{"data":{"events":[{"id":"e1","title":"T"}]}}}`)
	})
	raw, err := c.Call(context.Background(), "getEventsByIds", map[string]any{"params": map[string]any{"eventIds": []string{"e1"}}})
	if err != nil || !strings.Contains(string(raw), `"title":"T"`) {
		t.Fatalf("Call = %s, %v", raw, err)
	}
}

func TestCallErrorPayload(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		io.WriteString(w, `{"error":{"message":"User is host","status":"FAILED_PRECONDITION"}}`)
	})
	_, err := c.Call(context.Background(), "getGuests", nil)
	ae, ok := err.(*APIError)
	if !ok || ae.Code != "FAILED_PRECONDITION" || !strings.Contains(ae.Error(), "User is host") {
		t.Fatalf("err = %v", err)
	}
}

func TestUpdateEventPatchShape(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/fs/events/e1" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("currentDocument.exists") != "true" {
			t.Error("patch must require existing document")
		}
		mask := strings.Join(q["updateMask.fieldPaths"], ",")
		if mask != "`title`,`updatedBy`" {
			t.Errorf("mask = %s", mask)
		}
		var body struct {
			Fields map[string]map[string]any `json:"fields"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Fields["title"]["stringValue"] != "New" || !strings.HasSuffix(body.Fields["updatedBy"]["referenceValue"].(string), "/users/uid1") {
			t.Errorf("fields = %#v", body.Fields)
		}
		io.WriteString(w, `{"name":"projects/p/databases/(default)/documents/events/e1","fields":{"title":{"stringValue":"New"}}}`)
	})
	doc, err := c.UpdateEvent(context.Background(), "e1", map[string]any{"title": "New"})
	if err != nil || doc.ID != "e1" || doc.Fields["title"] != "New" {
		t.Fatalf("UpdateEvent = %+v, %v", doc, err)
	}
}

func TestListGuestsPaging(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pageToken") == "" {
			io.WriteString(w, `{"documents":[{"name":"x/guests/g1","fields":{"name":{"stringValue":"A"},"status":{"stringValue":"GOING"},"count":{"integerValue":"2"}}}],"nextPageToken":"p2"}`)
			return
		}
		io.WriteString(w, `{"documents":[{"name":"x/guests/g2","fields":{"name":{"stringValue":"B"},"status":{"stringValue":"MAYBE"},"user":{"referenceValue":"projects/p/databases/(default)/documents/users/u2"}}}]}`)
	})
	gs, err := c.ListGuests(context.Background(), "e1")
	if err != nil || len(gs) != 2 {
		t.Fatalf("ListGuests = %+v, %v", gs, err)
	}
	if gs[0].Name != "A" || gs[0].Count != 2 || gs[1].UserID != "u2" {
		t.Errorf("unexpected guests: %+v", gs)
	}
}

func TestRateLimitSurfacesTypedError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	_, err := c.GetDocument(context.Background(), "events/e1")
	if err == nil || !strings.Contains(err.Error(), "429") {
		t.Fatalf("want typed rate-limit error, got %v", err)
	}
}

func TestSessionSaveIsPrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s", "session.json")
	t.Setenv(SessionPathEnv, path)
	t.Setenv(RefreshTokenEnv, "")
	s, _ := NewSession("abc")
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSession()
	if err != nil || loaded.RefreshToken != "abc" {
		t.Fatalf("LoadSession = %+v, %v", loaded, err)
	}
	if fi, _ := filepath.Glob(path); len(fi) != 1 {
		t.Fatal("session file missing")
	}
	if info, _ := statMode(path); info != 0o600 {
		t.Errorf("session mode = %o, want 600", info)
	}
}
