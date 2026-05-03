package matches

import (
	"errors"
	"fmt"
	"testing"
	"time"

	profixio "github.com/nvbf/tournament-sync/repos/profixio"
)

func buildScoreEvents(startTS int64, count int, team string, prefix string) []Event {
	events := make([]Event, 0, count)
	for i := 0; i < count; i++ {
		events = append(events, Event{
			ID:        fmt.Sprintf("%s-score-%d", prefix, i),
			EventType: "SCORE",
			Team:      team,
			Timestamp: startTS + int64(i),
		})
	}
	return events
}

func buildValidTwoSetMatchEvents(startTS int64) []Event {
	events := []Event{}
	homeSetOne := buildScoreEvents(startTS, 21, "HOME", "set1")
	events = append(events, homeSetOne...)
	events = append(events, Event{ID: "set1-final", EventType: "SET_FINALIZED", Timestamp: startTS + 100})

	homeSetTwo := buildScoreEvents(startTS+200, 21, "HOME", "set2")
	events = append(events, homeSetTwo...)
	events = append(events, Event{ID: "set2-final", EventType: "SET_FINALIZED", Timestamp: startTS + 400})

	return events
}

// Test function to validate correct match results.
func TestValidateMatchResultCorrect(t *testing.T) {
	cases := []profixio.MatchResult{
		{
			Sets: []profixio.Result{
				{Home: 21, Away: 19},
				{Home: 29, Away: 31},
				{Home: 31, Away: 29},
			},
			Result: profixio.Result{Home: 2, Away: 1},
		},
		{
			Sets: []profixio.Result{
				{Home: 21, Away: 19},
				{Home: 23, Away: 21},
			},
			Result: profixio.Result{Home: 2, Away: 0},
		},
		{
			Sets: []profixio.Result{
				{Home: 21, Away: 0},
				{Home: 21, Away: 0},
			},
			Result: profixio.Result{Home: 2, Away: 0},
		},
		{
			Sets: []profixio.Result{
				{Home: 22, Away: 20},
				{Home: 21, Away: 23},
				{Home: 16, Away: 14},
			},
			Result: profixio.Result{Home: 2, Away: 1},
		},
	}

	for _, c := range cases {
		if !validateMatchResult(c) {
			t.Errorf("Expected match result to be valid, got invalid for %+v", c)
		}
	}
}

// Test function to validate incorrect match results.
func TestValidateMatchResultIncorrect(t *testing.T) {
	cases := []profixio.MatchResult{
		{
			Sets: []profixio.Result{
				{Home: 21, Away: 20}, // Not a valid ending score
				{Home: 23, Away: 21},
				{Home: 15, Away: 13},
			},
			Result: profixio.Result{Home: 2, Away: 1},
		},
		{
			Sets: []profixio.Result{
				{Home: 21, Away: 19},
				{Home: 21, Away: 23},
			},
			Result: profixio.Result{Home: 1, Away: 1}, // Match not done
		},
		{
			Sets: []profixio.Result{
				{Home: 21, Away: 19},
				{Home: 23, Away: 21},
				{Home: 13, Away: 15}, // Last set should not have been played
			},
			Result: profixio.Result{Home: 2, Away: 1},
		},
		{
			Sets: []profixio.Result{
				{Home: 21, Away: 19},
				{Home: 23, Away: 21},
				{Home: 0, Away: 0}, // Last set should not have been started
			},
			Result: profixio.Result{Home: 2, Away: 0},
		},
		{
			Sets: []profixio.Result{
				{Home: 21, Away: 19},
				{Home: 23, Away: 21},
				{Home: 15, Away: 13},
				{Home: 15, Away: 12}, // Too many sets
			},
			Result: profixio.Result{Home: 3, Away: 1},
		},
	}

	for _, c := range cases {
		if validateMatchResult(c) {
			t.Errorf("Expected match result to be invalid, got valid for %+v", c)
		}
	}
}

func TestValidateFinalizeCandidate(t *testing.T) {
	startTS := int64(1_700_000_000_000)
	validEvents := buildValidTwoSetMatchEvents(startTS)
	restoredScoreEvents := append(append([]Event{}, validEvents...),
		Event{ID: "undo-set2-last-score", EventType: "UNDO", Reference: "set2-score-20", Timestamp: startTS + 401},
		Event{ID: "undo-the-undo", EventType: "UNDO", Reference: "undo-set2-last-score", Timestamp: startTS + 402},
	)
	undoneFinalizeEvents := append(append([]Event{}, validEvents...),
		Event{ID: "match-final", EventType: "MATCH_FINALIZED", Timestamp: startTS + 500},
		Event{ID: "undo-match-final", EventType: "UNDO", Reference: "match-final", Timestamp: startTS + 501},
	)

	cases := []struct {
		name     string
		events   []Event
		now      time.Time
		expected error
	}{
		{
			name:     "valid and old enough",
			events:   validEvents,
			now:      time.UnixMilli(startTS + 400 + (6 * 60 * 1000)),
			expected: nil,
		},
		{
			name:     "too early",
			events:   validEvents,
			now:      time.UnixMilli(startTS + 400 + (4 * 60 * 1000)),
			expected: ErrFinalizeTooSoon,
		},
		{
			name:     "already finalized",
			events:   append(validEvents, Event{ID: "match-final", EventType: "MATCH_FINALIZED", Timestamp: startTS + 500}),
			now:      time.UnixMilli(startTS + 500 + (6 * 60 * 1000)),
			expected: ErrMatchAlreadyFinalized,
		},
		{
			name:     "finalize undone remains finalizable",
			events:   undoneFinalizeEvents,
			now:      time.UnixMilli(startTS + 501 + (6 * 60 * 1000)),
			expected: nil,
		},
		{
			name:     "undo of undo restores valid result",
			events:   restoredScoreEvents,
			now:      time.UnixMilli(startTS + 402 + (6 * 60 * 1000)),
			expected: nil,
		},
		{
			name: "invalid match result",
			events: []Event{
				{ID: "s1-1", EventType: "SCORE", Team: "HOME", Timestamp: startTS},
				{ID: "s1-final", EventType: "SET_FINALIZED", Timestamp: startTS + 1},
			},
			now:      time.UnixMilli(startTS + (6 * 60 * 1000)),
			expected: ErrInvalidMatchResult,
		},
		{
			name:     "no events",
			events:   []Event{},
			now:      time.UnixMilli(startTS + (6 * 60 * 1000)),
			expected: ErrNoEventsToFinalize,
		},
	}

	for _, c := range cases {
		err := validateFinalizeCandidate(c.events, c.now)
		if !errors.Is(err, c.expected) {
			t.Errorf("%s: expected error %v, got %v", c.name, c.expected, err)
		}
		if c.name == "too early" {
			tooSoonErr := &FinalizeTooSoonError{}
			if !errors.As(err, &tooSoonErr) {
				t.Errorf("%s: expected FinalizeTooSoonError metadata, got %T", c.name, err)
			}
		}
	}
}

func TestLatestEventTimeSupportsSecondsAndMilliseconds(t *testing.T) {
	cases := []struct {
		name     string
		events   []Event
		expected time.Time
	}{
		{
			name: "milliseconds",
			events: []Event{
				{Timestamp: 1_700_000_000_100},
				{Timestamp: 1_700_000_000_200},
			},
			expected: time.UnixMilli(1_700_000_000_200),
		},
		{
			name: "seconds",
			events: []Event{
				{Timestamp: 1_700_000_100},
				{Timestamp: 1_700_000_200},
			},
			expected: time.Unix(1_700_000_200, 0),
		},
	}

	for _, c := range cases {
		got := latestEventTime(c.events)
		if !got.Equal(c.expected) {
			t.Errorf("%s: expected %v, got %v", c.name, c.expected, got)
		}
	}
}
