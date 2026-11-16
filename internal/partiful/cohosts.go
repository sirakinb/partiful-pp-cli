// Copyright 2026 sirakinb and contributors. Licensed under Apache-2.0. See LICENSE.

package partiful

import (
	"context"
	"strings"
	"time"
)

// Co-hosts are added the way partiful.com's "Add co-hosts" modal does it: a
// createCohostRequest callable per person. The person is notified and becomes
// a co-host (listed in the event's owners) once they accept.

// CohostRequest is one co-host invitation on an event.
type CohostRequest struct {
	UserID    string `json:"userId"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt,omitempty"`
}

// Cohosts returns the event's co-host requests and its current owner ids.
func (c *Client) Cohosts(ctx context.Context, eventID string) ([]CohostRequest, []string, error) {
	doc, err := c.EventDocument(ctx, eventID)
	if err != nil {
		return nil, nil, err
	}
	docs, err := c.ListDocuments(ctx, "events/"+eventID+"/cohostRequests", 0)
	if err != nil {
		return nil, nil, err
	}
	reqs := make([]CohostRequest, 0, len(docs))
	for _, d := range docs {
		r := CohostRequest{UserID: d.ID}
		r.Status, _ = d.Fields["status"].(string)
		r.CreatedAt, _ = d.Fields["createdAt"].(string)
		reqs = append(reqs, r)
	}
	return reqs, OwnerIDs(doc), nil
}

// OwnerIDs extracts user ids from an event document's owners references.
func OwnerIDs(doc *Document) []string {
	refs, _ := doc.Fields["owners"].([]any)
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		s, _ := r.(string)
		if s != "" {
			out = append(out, s[strings.LastIndex(s, "/")+1:])
		}
	}
	return out
}

// RequestCohost invites a user to co-host an event.
func (c *Client) RequestCohost(ctx context.Context, eventID, userID string) error {
	_, err := c.Call(ctx, "createCohostRequest", map[string]any{"params": map[string]any{"eventId": eventID, "targetUserId": userID}})
	return err
}

// LatestQuestions returns the questions of an event's newest questionnaire
// version, as stored (id, text, type, required, options).
func LatestQuestions(doc *Document) []map[string]any {
	versions, _ := doc.Fields["questionnaireVersions"].([]any)
	if len(versions) == 0 {
		return nil
	}
	last, _ := versions[len(versions)-1].(map[string]any)
	qs, _ := last["questions"].([]any)
	out := make([]map[string]any, 0, len(qs))
	for _, q := range qs {
		if m, ok := q.(map[string]any); ok {
			clean := map[string]any{}
			for _, k := range []string{"id", "text", "type", "required", "options"} {
				if v, ok := m[k]; ok && v != nil {
					clean[k] = v
				}
			}
			out = append(out, clean)
		}
	}
	return out
}

// SetQuestionnaire saves a new questionnaire version and turns the
// questionnaire on, exactly like the questionnaire editor's Save button:
// questionnaireVersions gets the new version appended (earlier versions are
// kept so existing answers still resolve) and questionnaireEnabled is true.
func (c *Client) SetQuestionnaire(ctx context.Context, eventID string, questions []map[string]any) error {
	uid, err := c.UserID(ctx)
	if err != nil {
		return err
	}
	doc, err := c.EventDocument(ctx, eventID)
	if err != nil {
		return err
	}
	existing, _ := doc.Fields["questionnaireVersions"].([]any)
	versions := make([]any, 0, len(existing)+1)
	for _, v := range existing {
		versions = append(versions, restoreRefs(v))
	}
	qs := make([]any, 0, len(questions))
	for _, q := range questions {
		qs = append(qs, q)
	}
	versions = append(versions, map[string]any{
		"questions": qs,
		"createdAt": time.Now().UTC(),
		"createdBy": DocRef("users/" + uid),
	})
	_, err = c.UpdateEvent(ctx, eventID, map[string]any{"questionnaireVersions": versions, "questionnaireEnabled": true})
	return err
}

// restoreRefs turns decoded createdBy/createdAt strings in an existing
// questionnaire version back into a reference and timestamp so rewriting the
// array does not change their Firestore types.
func restoreRefs(v any) any {
	m, ok := v.(map[string]any)
	if !ok {
		return v
	}
	out := make(map[string]any, len(m))
	for k, val := range m {
		out[k] = val
	}
	if s, ok := m["createdBy"].(string); ok && strings.HasPrefix(s, "projects/") {
		out["createdBy"] = Ref(s)
	}
	if s, ok := m["createdAt"].(string); ok {
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			out["createdAt"] = t
		}
	}
	return out
}
