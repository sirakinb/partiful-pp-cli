// Copyright 2026 sirakinb and contributors. Licensed under Apache-2.0. See LICENSE.

package partiful

import (
	"context"
	"sort"
	"strings"
)

// Guest is one RSVP row from the host's guest list.
type Guest struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	Count       int64  `json:"count"`
	PlusOnes    int64  `json:"plusOnes"`
	RSVPDate    string `json:"rsvpDate,omitempty"`
	RSVPMessage string `json:"rsvpMessage,omitempty"`
	UserID      string `json:"userId,omitempty"`
}

// GoingStatuses are the statuses that count toward headcount.
var GoingStatuses = map[string]bool{"GOING": true, "APPROVED": true}

// ListGuests reads events/<id>/guests (hosts and cohosts only).
func (c *Client) ListGuests(ctx context.Context, eventID string) ([]Guest, error) {
	docs, err := c.ListDocuments(ctx, "events/"+eventID+"/guests", 0)
	if err != nil {
		return nil, err
	}
	out := make([]Guest, 0, len(docs))
	for _, d := range docs {
		g := Guest{ID: d.ID}
		g.Name, _ = d.Fields["name"].(string)
		g.Status, _ = d.Fields["status"].(string)
		g.Count, _ = d.Fields["count"].(int64)
		g.PlusOnes, _ = d.Fields["plusOneCount"].(int64)
		g.RSVPDate, _ = d.Fields["rsvpDate"].(string)
		g.RSVPMessage, _ = d.Fields["rsvpMessage"].(string)
		if ref, ok := d.Fields["user"].(string); ok {
			g.UserID = ref[strings.LastIndex(ref, "/")+1:]
		}
		out = append(out, g)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Status != out[j].Status {
			return out[i].Status < out[j].Status
		}
		return out[i].RSVPDate < out[j].RSVPDate
	})
	return out, nil
}

// GuestSummary aggregates guest rows by status.
type GuestSummary struct {
	EventID   string           `json:"eventId"`
	Total     int              `json:"totalRows"`
	Headcount int64            `json:"goingHeadcount"`
	ByStatus  map[string]int   `json:"byStatus"`
	PeopleBy  map[string]int64 `json:"peopleByStatus"`
}

// Summarize counts rows and people (count includes plus-ones) per status.
func Summarize(eventID string, guests []Guest) GuestSummary {
	s := GuestSummary{EventID: eventID, Total: len(guests), ByStatus: map[string]int{}, PeopleBy: map[string]int64{}}
	for _, g := range guests {
		s.ByStatus[g.Status]++
		people := g.Count
		if people <= 0 {
			people = 1 + g.PlusOnes
		}
		s.PeopleBy[g.Status] += people
		if GoingStatuses[g.Status] {
			s.Headcount += people
		}
	}
	return s
}
