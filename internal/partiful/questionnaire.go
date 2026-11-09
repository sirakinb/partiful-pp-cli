// Copyright 2026 sirakinb and contributors. Licensed under Apache-2.0. See LICENSE.

package partiful

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// The RSVP questionnaire lives in two places, which is also what the host's
// Questionnaire panel on partiful.com reads:
//
//   - the event document's questionnaireVersions[].questions (id, text, type)
//   - each guest document's questionnaireResponse.answers, keyed by question id

// Question is one questionnaire question.
type Question struct {
	ID       string   `json:"id"`
	Text     string   `json:"text"`
	Type     string   `json:"type"`
	Required bool     `json:"required"`
	Options  []string `json:"options,omitempty"`
}

// Answer is one guest's answer to one question.
type Answer struct {
	QuestionID string `json:"questionId"`
	Question   string `json:"question"`
	Type       string `json:"type"`
	Answer     string `json:"answer"`
}

// QuestionnaireResponse is one guest's RSVP plus their answers.
type QuestionnaireResponse struct {
	GuestID  string   `json:"guestId"`
	Name     string   `json:"name"`
	Status   string   `json:"status"`
	Answered bool     `json:"answered"`
	Email    string   `json:"email,omitempty"`
	Answers  []Answer `json:"answers"`
}

// Questionnaire returns the event's questions (latest version first in order
// of appearance) and every host-visible guest's answers.
func (c *Client) Questionnaire(ctx context.Context, eventID string) ([]Question, []QuestionnaireResponse, error) {
	doc, err := c.EventDocument(ctx, eventID)
	if err != nil {
		return nil, nil, err
	}
	questions := questionsFromVersions(doc.Fields["questionnaireVersions"])
	docs, err := c.ListDocuments(ctx, "events/"+eventID+"/guests", 0)
	if err != nil {
		return nil, nil, err
	}
	return questions, responsesFromGuests(questions, docs), nil
}

// questionsFromVersions merges all questionnaire versions; question ids are
// unique, and the newest version's text wins.
func questionsFromVersions(v any) []Question {
	versions, _ := v.([]any)
	byID := map[string]Question{}
	var order []string
	for i := len(versions) - 1; i >= 0; i-- {
		ver, _ := versions[i].(map[string]any)
		qs, _ := ver["questions"].([]any)
		for _, raw := range qs {
			q, _ := raw.(map[string]any)
			id := fmt.Sprint(q["id"])
			if _, seen := byID[id]; seen || id == "" || id == "<nil>" {
				continue
			}
			question := Question{ID: id}
			question.Text, _ = q["text"].(string)
			question.Type, _ = q["type"].(string)
			question.Required, _ = q["required"].(bool)
			if opts, ok := q["options"].([]any); ok {
				for _, o := range opts {
					question.Options = append(question.Options, fmt.Sprint(o))
				}
			}
			byID[id] = question
			order = append(order, id)
		}
	}
	out := make([]Question, 0, len(order))
	for _, id := range order {
		out = append(out, byID[id])
	}
	return out
}

// IsEmailQuestion reports whether a question collects an email address.
func IsEmailQuestion(q Question) bool {
	return q.Type == "email" || (q.Type != "select" && strings.Contains(strings.ToLower(q.Text), "email"))
}

func responsesFromGuests(questions []Question, docs []*Document) []QuestionnaireResponse {
	out := make([]QuestionnaireResponse, 0, len(docs))
	for _, d := range docs {
		r := QuestionnaireResponse{GuestID: d.ID, Answers: []Answer{}}
		r.Name, _ = d.Fields["name"].(string)
		r.Status, _ = d.Fields["status"].(string)
		resp, _ := d.Fields["questionnaireResponse"].(map[string]any)
		answers, _ := resp["answers"].(map[string]any)
		r.Answered = len(answers) > 0
		for _, q := range questions {
			raw, ok := answers[q.ID]
			if !ok || raw == nil {
				continue
			}
			val := answerString(raw)
			r.Answers = append(r.Answers, Answer{QuestionID: q.ID, Question: q.Text, Type: q.Type, Answer: val})
			if r.Email == "" && IsEmailQuestion(q) {
				r.Email = strings.TrimSpace(val)
			}
		}
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Answered != out[j].Answered {
			return out[i].Answered
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// answerString flattens an answer value (text, choice, or list of choices).
func answerString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []any:
		parts := make([]string, 0, len(x))
		for _, e := range x {
			parts = append(parts, fmt.Sprint(e))
		}
		return strings.Join(parts, "; ")
	default:
		return fmt.Sprint(x)
	}
}

// Emails returns the unique, non-empty email answers in response order.
func Emails(responses []QuestionnaireResponse) []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range responses {
		e := strings.TrimSpace(r.Email)
		if e == "" || seen[strings.ToLower(e)] {
			continue
		}
		seen[strings.ToLower(e)] = true
		out = append(out, e)
	}
	return out
}
