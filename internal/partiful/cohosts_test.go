// Copyright 2026 sirakinb and contributors. Licensed under Apache-2.0. See LICENSE.

package partiful

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestOwnerIDsAndLatestQuestions(t *testing.T) {
	doc := &Document{Fields: map[string]any{
		"owners": []any{"projects/p/databases/(default)/documents/users/me", "projects/p/databases/(default)/documents/users/co"},
		"questionnaireVersions": []any{
			map[string]any{"questions": []any{map[string]any{"id": "old", "text": "Old", "type": "short_answer"}}},
			map[string]any{"createdBy": "projects/p/x", "questions": []any{
				map[string]any{"id": "q1", "text": "Email?", "type": "email", "required": true, "extra": "drop me"},
			}},
		},
	}}
	if got := OwnerIDs(doc); len(got) != 2 || got[1] != "co" {
		t.Fatalf("OwnerIDs = %v", got)
	}
	qs := LatestQuestions(doc)
	if len(qs) != 1 || qs[0]["id"] != "q1" || qs[0]["required"] != true {
		t.Fatalf("LatestQuestions = %v", qs)
	}
	if _, ok := qs[0]["extra"]; ok {
		t.Error("unknown keys should be dropped")
	}
	if LatestQuestions(&Document{Fields: map[string]any{}}) != nil {
		t.Error("no versions should return nil")
	}
}

func TestRequestCohostEnvelope(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Data struct {
				Params map[string]any `json:"params"`
			} `json:"data"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if r.URL.Path != "/api/createCohostRequest" || body.Data.Params["eventId"] != "e1" || body.Data.Params["targetUserId"] != "u2" {
			t.Errorf("got %s %v", r.URL.Path, body.Data.Params)
		}
		io.WriteString(w, `{"result":{"data":{"id":"u2"}}}`)
	})
	if err := c.RequestCohost(context.Background(), "e1", "u2"); err != nil {
		t.Fatal(err)
	}
}

func TestSetQuestionnaireAppendsVersion(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			io.WriteString(w, `{"name":"x/events/e1","fields":{"questionnaireVersions":{"arrayValue":{"values":[{"mapValue":{"fields":{"createdBy":{"referenceValue":"projects/getpartiful/databases/(default)/documents/users/uid1"},"createdAt":{"timestampValue":"2026-10-21T04:59:00.840Z"},"questions":{"arrayValue":{"values":[]}}}}}]}}}}`)
			return
		}
		mask := strings.Join(r.URL.Query()["updateMask.fieldPaths"], ",")
		if mask != "`questionnaireEnabled`,`questionnaireVersions`,`updatedBy`" {
			t.Errorf("mask = %s", mask)
		}
		b, _ := io.ReadAll(r.Body)
		s := string(b)
		for _, want := range []string{`"booleanValue":true`, `"referenceValue":"projects/getpartiful/databases/(default)/documents/users/uid1"`, `"timestampValue":"2026-10-21T04:59:00.84Z"`, `"stringValue":"Email?"`} {
			if !strings.Contains(s, want) {
				t.Errorf("patch body missing %s", want)
			}
		}
		var body struct {
			Fields map[string]map[string]any `json:"fields"`
		}
		_ = json.Unmarshal(b, &body)
		vals := body.Fields["questionnaireVersions"]["arrayValue"].(map[string]any)["values"].([]any)
		if len(vals) != 2 {
			t.Errorf("want existing version kept plus new one, got %d", len(vals))
		}
		io.WriteString(w, `{"name":"x/events/e1","fields":{}}`)
	})
	_ = time.Now
	err := c.SetQuestionnaire(context.Background(), "e1", []map[string]any{{"id": "q1", "text": "Email?", "type": "email", "required": true}})
	if err != nil {
		t.Fatal(err)
	}
}
