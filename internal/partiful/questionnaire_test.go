// Copyright 2026 sirakinb and contributors. Licensed under Apache-2.0. See LICENSE.

package partiful

import "testing"

func TestQuestionnaireJoin(t *testing.T) {
	versions := []any{
		map[string]any{"questions": []any{
			map[string]any{"id": "q1", "text": "Old wording", "type": "short_answer"},
		}},
		map[string]any{"questions": []any{
			map[string]any{"id": "q1", "text": "Experience?", "type": "select", "options": []any{"New", "Pro"}},
			map[string]any{"id": "q2", "text": "What is your email address?", "type": "email", "required": true},
		}},
	}
	qs := questionsFromVersions(versions)
	if len(qs) != 2 || qs[0].Text != "Experience?" || !IsEmailQuestion(qs[1]) || IsEmailQuestion(qs[0]) {
		t.Fatalf("questions = %+v", qs)
	}
	docs := []*Document{
		{ID: "g1", Fields: map[string]any{"name": "Zed", "status": "GOING", "questionnaireResponse": map[string]any{
			"answers": map[string]any{"q1": "Pro", "q2": " zed@example.com "}}}},
		{ID: "g2", Fields: map[string]any{"name": "amy", "status": "MAYBE", "questionnaireResponse": map[string]any{
			"answers": map[string]any{"q1": []any{"New", "Pro"}, "q2": "ZED@example.com"}}}},
		{ID: "g3", Fields: map[string]any{"name": "Bo", "status": "GOING"}},
	}
	rs := responsesFromGuests(qs, docs)
	if len(rs) != 3 || !rs[0].Answered || rs[0].Name != "amy" || rs[2].Answered {
		t.Fatalf("order/answered wrong: %+v", rs)
	}
	if rs[0].Answers[0].Answer != "New; Pro" || rs[1].Email != "zed@example.com" {
		t.Errorf("answers = %+v / %+v", rs[0].Answers, rs[1])
	}
	if got := Emails(rs); len(got) != 1 {
		t.Errorf("emails should dedupe case-insensitively, got %v", got)
	}
}
