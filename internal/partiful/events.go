// Copyright 2026 sirakinb and contributors. Licensed under Apache-2.0. See LICENSE.

package partiful

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

var eventIDPattern = regexp.MustCompile(`^[A-Za-z0-9]{12,40}$`)

// ParseEventID accepts a bare event id or a partiful.com/e/<id> URL.
func ParseEventID(s string) (string, error) {
	s = strings.TrimSpace(s)
	if strings.Contains(s, "/") {
		u, err := url.Parse(s)
		if err == nil {
			parts := strings.Split(strings.Trim(u.Path, "/"), "/")
			for i := 0; i+1 < len(parts); i++ {
				if parts[i] == "e" {
					s = parts[i+1]
					break
				}
			}
		}
	}
	if !eventIDPattern.MatchString(s) {
		return "", fmt.Errorf("%q is not a Partiful event id or https://partiful.com/e/<id> URL", s)
	}
	return s, nil
}

// EventURL is the public link for an event.
func EventURL(id string) string { return "https://partiful.com/e/" + id }

// EventSummary is the normalized row used by `events list`.
type EventSummary struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	StartDate  string `json:"startDate,omitempty"`
	EndDate    string `json:"endDate,omitempty"`
	Timezone   string `json:"timezone,omitempty"`
	Status     string `json:"status,omitempty"`
	Role       string `json:"role"`
	When       string `json:"when"`
	Location   string `json:"location,omitempty"`
	GoingCount int    `json:"goingCount"`
	GuestCount int    `json:"guestCount"`
	IsPublic   bool   `json:"isPublic"`
	URL        string `json:"url"`
}

type homeEvent struct {
	ID                string         `json:"id"`
	Title             string         `json:"title"`
	StartDate         any            `json:"startDate"`
	EndDate           any            `json:"endDate"`
	Timezone          string         `json:"timezone"`
	Status            string         `json:"status"`
	OwnerIDs          []string       `json:"ownerIds"`
	Location          any            `json:"location"`
	GuestCount        *int           `json:"guestCount"`
	IsPublic          bool           `json:"isPublic"`
	GuestStatusCounts map[string]int `json:"guestStatusCounts"`
}

func dateString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case map[string]any:
		// Some home-page payloads serialize dates as {_seconds, _nanoseconds}.
		if s, ok := x["_seconds"].(float64); ok {
			return time.Unix(int64(s), 0).UTC().Format(time.RFC3339)
		}
	}
	return ""
}

func (h homeEvent) summary(uid, when string) EventSummary {
	role := "guest"
	for _, o := range h.OwnerIDs {
		if o == uid {
			role = "host"
		}
	}
	gc := 0
	if h.GuestCount != nil {
		gc = *h.GuestCount
	}
	loc, _ := h.Location.(string)
	return EventSummary{
		ID: h.ID, Title: h.Title, StartDate: dateString(h.StartDate), EndDate: dateString(h.EndDate),
		Timezone: h.Timezone, Status: h.Status, Role: role, When: when, Location: loc,
		GoingCount: h.GuestStatusCounts["GOING"], GuestCount: gc, IsPublic: h.IsPublic, URL: EventURL(h.ID),
	}
}

// ListEvents returns upcoming and/or past events visible on the user's home page.
func (c *Client) ListEvents(ctx context.Context, upcoming, past bool) ([]EventSummary, error) {
	uid, err := c.UserID(ctx)
	if err != nil {
		return nil, err
	}
	var out []EventSummary
	if upcoming {
		raw, err := c.Call(ctx, "getMyUpcomingEventsForHomePage", nil)
		if err != nil {
			return nil, err
		}
		var d struct {
			UpcomingEvents []homeEvent `json:"upcomingEvents"`
		}
		if err := json.Unmarshal(raw, &d); err != nil {
			return nil, err
		}
		for _, e := range d.UpcomingEvents {
			out = append(out, e.summary(uid, "upcoming"))
		}
	}
	if past {
		raw, err := c.Call(ctx, "getMyPastEventsForHomePage", nil)
		if err != nil {
			return nil, err
		}
		var d struct {
			PastEvents []homeEvent `json:"pastEvents"`
		}
		if err := json.Unmarshal(raw, &d); err != nil {
			return nil, err
		}
		for _, e := range d.PastEvents {
			out = append(out, e.summary(uid, "past"))
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].When != out[j].When {
			return out[i].When == "upcoming"
		}
		if out[i].When == "upcoming" {
			return out[i].StartDate < out[j].StartDate
		}
		return out[i].StartDate > out[j].StartDate
	})
	return out, nil
}

// EditableFields are the event document fields `events update` can change,
// in display order.
var EditableFields = []string{"title", "startDate", "endDate", "timezone", "description", "isPublic", "image"}

// GetEvent merges the callable view (counts, public fields) with the
// Firestore document (full host view) when the user can read it.
func (c *Client) GetEvent(ctx context.Context, id string) (map[string]any, error) {
	raw, err := c.Call(ctx, "getEventsByIds", map[string]any{"params": map[string]any{"eventIds": []string{id}}})
	if err != nil {
		return nil, err
	}
	var d struct {
		Events []map[string]any `json:"events"`
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, err
	}
	if len(d.Events) == 0 {
		return nil, &APIError{Status: 404, Code: "NOT_FOUND", Message: "event " + id + " not found or not visible to you", Surface: "getEventsByIds"}
	}
	ev := d.Events[0]
	if doc, err := c.GetDocument(ctx, "events/"+id); err == nil {
		for _, k := range []string{"description", "location", "locationInfo", "visibility", "maxCapacity", "enableWaitlist", "rsvpsEnabled", "publicShortUrl", "createdAt", "publishedAt", "guestStatusCounts"} {
			if v, ok := doc.Fields[k]; ok {
				ev[k] = v
			}
		}
	}
	ev["url"] = EventURL(id)
	return ev, nil
}

// EventDocument reads the raw Firestore event document (host view).
func (c *Client) EventDocument(ctx context.Context, id string) (*Document, error) {
	return c.GetDocument(ctx, "events/"+id)
}

// UpdateEvent patches the given fields on events/<id> the same way the web
// app does (updateDoc with updatedBy set to the current user).
func (c *Client) UpdateEvent(ctx context.Context, id string, fields map[string]any) (*Document, error) {
	uid, err := c.UserID(ctx)
	if err != nil {
		return nil, err
	}
	patch := make(map[string]any, len(fields)+1)
	for k, v := range fields {
		patch[k] = v
	}
	patch["updatedBy"] = DocRef("users/" + uid)
	return c.PatchDocument(ctx, "events/"+id, patch)
}

// SetLocation sets a free-text location (what you would type in the web app).
func (c *Client) SetLocation(ctx context.Context, id, location string) error {
	_, err := c.Call(ctx, "setEventLocation", map[string]any{"params": map[string]any{
		"eventId":      id,
		"locationInfo": FreeformLocation(location),
	}})
	return err
}

// FreeformLocation is the locationInfo payload for typed-in addresses.
func FreeformLocation(location string) map[string]any {
	return map[string]any{"type": "freeform", "value": location}
}

// CreateEvent calls createEvent with a full event object and returns the new id.
func (c *Client) CreateEvent(ctx context.Context, event map[string]any) (string, error) {
	raw, err := c.Call(ctx, "createEvent", map[string]any{"params": map[string]any{"event": event, "cohostIds": []string{}}})
	if err != nil {
		return "", err
	}
	var id string
	if json.Unmarshal(raw, &id) == nil && id != "" {
		return id, nil
	}
	var obj map[string]any
	if json.Unmarshal(raw, &obj) == nil {
		for _, k := range []string{"eventId", "id"} {
			if s, ok := obj[k].(string); ok && s != "" {
				return s, nil
			}
		}
	}
	return "", fmt.Errorf("createEvent returned an unexpected payload: %s", strings.TrimSpace(string(raw)))
}

// PublishEvent publishes a draft event.
func (c *Client) PublishEvent(ctx context.Context, id string) error {
	_, err := c.Call(ctx, "publishEvent", map[string]any{"params": map[string]any{"eventId": id}})
	return err
}

// NewEventDefaults mirrors the defaults partiful.com/create starts from.
func NewEventDefaults(timezone string) map[string]any {
	return map[string]any{
		"showHostList":               true,
		"showGuestCount":             true,
		"showGuestList":              true,
		"showActivityTimestamps":     true,
		"displayInviteButton":        true,
		"visibility":                 "public",
		"allowGuestPhotoUpload":      true,
		"enableGuestReminders":       true,
		"rsvpsEnabled":               true,
		"allowGuestsToInviteMutuals": true,
		"isPublic":                   false,
		"timezone":                   timezone,
	}
}

// TemplateFields are copied from an existing event by `events create --copy-from`.
var TemplateFields = []string{"description", "image", "displaySettings", "rsvpButtonGlyphType", "showHostList", "showGuestCount", "showGuestList",
	"showActivityTimestamps", "displayInviteButton", "allowGuestPhotoUpload", "enableGuestReminders", "allowGuestsToInviteMutuals", "timezone", "isPublic", "visibility"}

// ParseTime accepts RFC3339 or "YYYY-MM-DD HH:MM" (interpreted in tz).
func ParseTime(s, tz string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.Time{}, fmt.Errorf("unknown timezone %q", tz)
	}
	for _, layout := range []string{"2006-01-02 15:04", "2006-01-02T15:04", "2006-01-02 3:04pm", "2006-01-02 3pm"} {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse time %q: use \"2026-11-14 09:00\" or RFC3339", s)
}
