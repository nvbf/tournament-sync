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
		{
			name: "real match with complex undo chains",
			events: []Event{
				{Author: "Jed5i0gGbLUZaMhPHH7Wx2p8Heb2", EventType: "FIRST_TEAM_SERVE", ID: "8224956b-dfe4-40f9-9270-6b42be3a91a0", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777833898549, Undone: ""},
				{Author: "Jed5i0gGbLUZaMhPHH7Wx2p8Heb2", EventType: "FIRST_PLAYER_SERVE", ID: "9fa08d3f-b13d-4367-8c8f-b78a2a00f194", PlayerID: 1, Reference: "", Team: "HOME", Timestamp: 1777833899225, Undone: ""},
				{Author: "Jed5i0gGbLUZaMhPHH7Wx2p8Heb2", EventType: "FIRST_PLAYER_SERVE", ID: "b37b8cfe-3c8b-471e-9488-94f3459e3f5e", PlayerID: 1, Reference: "", Team: "AWAY", Timestamp: 1777833899962, Undone: ""},
				{Author: "Jed5i0gGbLUZaMhPHH7Wx2p8Heb2", EventType: "LEFT_SIDE_START_TEAM", ID: "7432762e-43d3-4ec8-bd28-5ae5f2bcd297", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777833901770, Undone: ""},
				{Author: "Jed5i0gGbLUZaMhPHH7Wx2p8Heb2", EventType: "SCORE", ID: "5f40d799-394e-426f-a2cf-d020df64c7fd", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777833903295, Undone: ""},
				{Author: "Jed5i0gGbLUZaMhPHH7Wx2p8Heb2", EventType: "SCORE", ID: "a24dbfd0-4c52-4ba8-8edb-0d0fdcb59885", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777833913456, Undone: ""},
				{Author: "Jed5i0gGbLUZaMhPHH7Wx2p8Heb2", EventType: "SCORE", ID: "e24901b2-34c6-47a4-9eee-81d4b041931e", PlayerID: 0, Reference: "", Team: "AWAY", Timestamp: 1777833929425, Undone: ""},
				{Author: "Jed5i0gGbLUZaMhPHH7Wx2p8Heb2", EventType: "SCORE", ID: "90f6c92b-e52a-48d7-91fb-27a2c38d9ce2", PlayerID: 0, Reference: "", Team: "AWAY", Timestamp: 1777833931820, Undone: ""},
				{Author: "Jed5i0gGbLUZaMhPHH7Wx2p8Heb2", EventType: "SCORE", ID: "390461a9-986d-4377-818a-b2a7e7126807", PlayerID: 0, Reference: "", Team: "AWAY", Timestamp: 1777833933054, Undone: ""},
				{Author: "Jed5i0gGbLUZaMhPHH7Wx2p8Heb2", EventType: "SCORE", ID: "02e3ce4e-8f46-4093-bc24-6bc7106c7376", PlayerID: 0, Reference: "", Team: "AWAY", Timestamp: 1777833935990, Undone: ""},
				{Author: "Jed5i0gGbLUZaMhPHH7Wx2p8Heb2", EventType: "SCORE", ID: "d8df7d2b-a517-49ab-a8b7-cbe8a741177f", PlayerID: 0, Reference: "", Team: "AWAY", Timestamp: 1777833937843, Undone: ""},
				{Author: "Jed5i0gGbLUZaMhPHH7Wx2p8Heb2", EventType: "CLEAR_MESSAGE", ID: "e48cb2df-d9db-4dc7-abf5-84476446cccc", PlayerID: 0, Reference: "", Team: "NONE", Timestamp: 1777833990192, Undone: ""},
				{Author: "Jed5i0gGbLUZaMhPHH7Wx2p8Heb2", EventType: "SCORE", ID: "4a107536-9c5a-4f80-8196-03c29b48334d", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777833991499, Undone: ""},
				{Author: "Jed5i0gGbLUZaMhPHH7Wx2p8Heb2", EventType: "SCORE", ID: "8805f408-994f-445d-b576-42de527b05ed", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777833994068, Undone: ""},
				{Author: "Jed5i0gGbLUZaMhPHH7Wx2p8Heb2", EventType: "SCORE", ID: "3b478718-d2bc-4c39-99c1-82d1ed5a89f2", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777833994392, Undone: ""},
				{Author: "Jed5i0gGbLUZaMhPHH7Wx2p8Heb2", EventType: "SCORE", ID: "1409f232-5aaa-4d42-b56d-a7ebfcb4c9ad", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777833994719, Undone: ""},
				{Author: "Jed5i0gGbLUZaMhPHH7Wx2p8Heb2", EventType: "SCORE", ID: "523b5cc1-2030-4c53-95f6-469b70860d11", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777833994973, Undone: ""},
				{Author: "Jed5i0gGbLUZaMhPHH7Wx2p8Heb2", EventType: "SCORE", ID: "cad35c51-0c74-481d-ba47-0a3fb424ef19", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777833995240, Undone: ""},
				{Author: "Jed5i0gGbLUZaMhPHH7Wx2p8Heb2", EventType: "SCORE", ID: "9b1fd344-517f-47e5-b5e6-e8bb464650ea", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777833996144, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "CLEAR_MESSAGE", ID: "5cdc4207-ec7c-4e68-8a25-e7b955d15d88", PlayerID: 0, Reference: "", Team: "NONE", Timestamp: 1777835174318, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "89a94f49-57d0-4e06-aad2-0c2215cc76a6", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777835177726, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "887ed4db-196c-484f-999f-570cb586fec7", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777835177892, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "f4eaa391-dfe4-4974-bf30-5bab2255bfff", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777835178072, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "f0d1911c-25c9-4d06-a8a0-ac2545216b3f", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777835178237, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "055f1b0e-462b-4757-9210-b868cf76998c", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777835178384, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "1600da81-e505-4dae-856c-3062a46742e7", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777835178831, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "456fcdf9-7168-4d6d-b2c5-25b3131a8417", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777835180488, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "CLEAR_MESSAGE", ID: "f18a7cae-0337-415d-b7ca-f807ccdd9da2", PlayerID: 0, Reference: "", Team: "NONE", Timestamp: 1777835181613, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "d5adf162-dbef-4ccb-812a-2043aced8dc3", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777835182168, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "5ec3c01f-d452-4442-88b3-58f64dd9dba4", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777835182334, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "12739daf-05dd-4ac3-a7b0-2a242a2abda9", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777835182505, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "5f85674e-5406-489f-ad1e-95b9ac63dbcb", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777835182687, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "59a5f1ab-7739-4649-9559-1abb513473d2", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777835183107, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SET_FINALIZED", ID: "d75b8ba5-f466-4481-b039-d98d27ee98ee", PlayerID: 0, Reference: "", Team: "NONE", Timestamp: 1777835184224, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "FIRST_PLAYER_SERVE", ID: "165a6528-cc44-4c9d-920e-7130e3430ee4", PlayerID: 1, Reference: "", Team: "HOME", Timestamp: 1777835185306, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "FIRST_TEAM_SERVE", ID: "cb4e6394-61bd-4b6e-b6b6-82f54b1b2cfa", PlayerID: 0, Reference: "", Team: "AWAY", Timestamp: 1777835185517, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "FIRST_PLAYER_SERVE", ID: "abdd5a7f-d52d-44d1-ab22-3c2523c8ee42", PlayerID: 1, Reference: "", Team: "AWAY", Timestamp: 1777835185961, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "LEFT_SIDE_START_TEAM", ID: "7d70b107-963a-4e2c-82a5-3863da0fd23f", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777835190988, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "f9e9c3f6-9e9e-4748-a87f-740dfeacb938", PlayerID: 0, Reference: "", Team: "AWAY", Timestamp: 1777835191856, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "9f61278a-0246-4f3f-8b8d-e4e62b86eccf", PlayerID: 0, Reference: "", Team: "AWAY", Timestamp: 1777835192015, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "a0a82f55-1973-4032-8984-3bc50ccdd697", PlayerID: 0, Reference: "", Team: "AWAY", Timestamp: 1777835192158, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "7b0bcd0c-aa56-470e-9919-faf393d9e9b1", PlayerID: 0, Reference: "", Team: "AWAY", Timestamp: 1777835192327, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "2c318605-150d-4130-8979-f3fac1ae10ff", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777835192587, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "ab36b9ec-8526-4eba-83c0-edbffd59e3e2", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777835192778, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "549b4f8d-d152-407c-a55e-c1ff62ffddef", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777835192925, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "CLEAR_MESSAGE", ID: "b0ae0a15-de60-4803-b90b-1eb024d450d4", PlayerID: 0, Reference: "", Team: "NONE", Timestamp: 1777835193737, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "99e8a721-4adc-4434-acca-2cc5168d064e", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777835194196, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "af3d4e6e-4039-4f96-8ddd-243bfeb7cd5c", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777835194349, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "fa107929-65b7-4329-b064-799631f455b8", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777835194530, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "d9d84afa-15a8-4248-8c40-d402d204bc41", PlayerID: 0, Reference: "", Team: "AWAY", Timestamp: 1777835194783, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "6b972fc7-a7e7-45d3-a60e-67df877e0543", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777835195018, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "441120e1-dbfa-413c-91ec-4ab62e217473", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777835195179, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "b6dc5466-2d74-4880-a7c4-8f587908b352", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777835195336, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "CLEAR_MESSAGE", ID: "63211947-7c1d-4d0c-b848-911b95bcfa8a", PlayerID: 0, Reference: "", Team: "NONE", Timestamp: 1777835196177, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "6a863fb5-accd-4c5d-a77a-ec3cb5ef3972", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777835196691, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "75e1e199-3ab0-4a5c-8858-a6356b4eb608", PlayerID: 0, Reference: "", Team: "AWAY", Timestamp: 1777835196913, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "81abfcec-1f09-4b68-bb9c-df724027fc20", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777835197204, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "70e3a86b-f55f-4c3a-9d43-c232493f9cde", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777835197335, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "3f16994f-394a-40de-a51c-5ba95a6395d5", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777835197476, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "dea0b982-4a8e-4c40-b37c-ee2f7a1a3fd1", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777835197941, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "62b35776-7894-4f3f-8216-073497950016", PlayerID: 0, Reference: "", Team: "AWAY", Timestamp: 1777835198306, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "CLEAR_MESSAGE", ID: "dff4d4b7-a855-4b69-9ced-6f962caf64bf", PlayerID: 0, Reference: "", Team: "NONE", Timestamp: 1777835199056, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "519b2f3f-3ed8-483b-b082-f5967378f467", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777835199513, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "3d496939-0f70-425c-8ea1-9f50e66860eb", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777835199674, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "89a34ee2-425f-4c74-8ee5-cd1ee985f0d6", PlayerID: 0, Reference: "", Team: "AWAY", Timestamp: 1777835200124, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "989b68e7-8af7-4648-ba8e-99d88610dc2a", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777835200355, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "561e4773-bb19-4914-8566-42e7dc37144e", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777835200550, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "55eb1164-b9e5-4a96-a77e-2b6ddd16f7df", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777835201013, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "a4c5f0d0-6e84-4622-b656-7b794717dbd8", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777835201527, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "CLEAR_MESSAGE", ID: "c80662c9-0c95-4651-9aa2-59d5989dd5fc", PlayerID: 0, Reference: "", Team: "NONE", Timestamp: 1777835202331, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "b8095c02-7767-4462-96d0-5f9628e88d50", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777835202925, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "UNDO", ID: "b382e46f-0e7c-48b8-b990-2075b05a7f0f", PlayerID: 0, Reference: "b8095c02-7767-4462-96d0-5f9628e88d50", Team: "NONE", Timestamp: 1777838890277, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "UNDO", ID: "b635de9f-50ce-43af-bd37-ec58093d8157", PlayerID: 0, Reference: "c80662c9-0c95-4651-9aa2-59d5989dd5fc", Team: "NONE", Timestamp: 1777838903830, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "CLEAR_MESSAGE", ID: "4d2ec4fb-cdd1-4ab1-8669-fdae31fcf7ce", PlayerID: 0, Reference: "", Team: "NONE", Timestamp: 1777838908281, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "c909dc4b-99b1-44ca-8586-23e9d844ce3a", PlayerID: 0, Reference: "", Team: "AWAY", Timestamp: 1777838909017, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "be97059b-1838-4da1-81b7-53c2702b874c", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777838919153, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "UNDO", ID: "8c3c385a-a31c-4751-8716-86e53bd19581", PlayerID: 0, Reference: "be97059b-1838-4da1-81b7-53c2702b874c", Team: "NONE", Timestamp: 1777839744398, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "UNDO", ID: "3268ee21-6045-459f-b925-b30a2bdce9cf", PlayerID: 0, Reference: "c909dc4b-99b1-44ca-8586-23e9d844ce3a", Team: "NONE", Timestamp: 1777839745478, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "ad40b6a7-b0f2-47ba-b6cf-22151ae39526", PlayerID: 0, Reference: "", Team: "AWAY", Timestamp: 1777839746599, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "3b74ce76-308b-4a21-ab87-ed7839c30381", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777839758467, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "UNDO", ID: "65f17bbd-59c8-4082-9cd2-fd4469e6d9b6", PlayerID: 0, Reference: "3b74ce76-308b-4a21-ab87-ed7839c30381", Team: "NONE", Timestamp: 1777839882077, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "UNDO", ID: "20b7e189-1314-4233-b610-e7fcafa975ea", PlayerID: 0, Reference: "ad40b6a7-b0f2-47ba-b6cf-22151ae39526", Team: "NONE", Timestamp: 1777839883142, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "d7afec10-5bb7-48ec-a5fc-cea302badf11", PlayerID: 0, Reference: "", Team: "AWAY", Timestamp: 1777839884567, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "9fc5fe6d-f2b4-42ab-98b2-42695432570f", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777839886797, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "UNDO", ID: "5bce0b87-da60-4328-95a7-9e4a6a9d4da6", PlayerID: 0, Reference: "9fc5fe6d-f2b4-42ab-98b2-42695432570f", Team: "NONE", Timestamp: 1777840193893, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "UNDO", ID: "61e4085d-0cf3-493c-957f-f1d2e57dc13e", PlayerID: 0, Reference: "d7afec10-5bb7-48ec-a5fc-cea302badf11", Team: "NONE", Timestamp: 1777840195037, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "e40de817-aa13-4057-8a85-2f04c72ca47e", PlayerID: 0, Reference: "", Team: "AWAY", Timestamp: 1777840195986, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "f3266c6c-1591-48bd-b2c0-0ccb1ec0d4cc", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777840196795, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "UNDO", ID: "15f93222-6fac-45c9-b68c-c16e766c87ed", PlayerID: 0, Reference: "f3266c6c-1591-48bd-b2c0-0ccb1ec0d4cc", Team: "NONE", Timestamp: 1777840706944, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "UNDO", ID: "a1e94cdb-4a19-4077-8884-8233c9ee68f3", PlayerID: 0, Reference: "e40de817-aa13-4057-8a85-2f04c72ca47e", Team: "NONE", Timestamp: 1777840707238, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "1a13675b-8383-49ec-be81-3750e84764f3", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777840708014, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "UNDO", ID: "0e17e1e5-4953-4570-8bb4-752fb37458a3", PlayerID: 0, Reference: "1a13675b-8383-49ec-be81-3750e84764f3", Team: "NONE", Timestamp: 1777840709566, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "9d36cd19-0694-4e79-ad48-e5678663030c", PlayerID: 0, Reference: "", Team: "AWAY", Timestamp: 1777840710253, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "a7b1be68-e8d6-4b67-9a62-8795847e39e1", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777840710975, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "UNDO", ID: "7ae7d8fa-3c24-4ec6-9fd0-31068e77d651", PlayerID: 0, Reference: "a7b1be68-e8d6-4b67-9a62-8795847e39e1", Team: "NONE", Timestamp: 1777841681165, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "UNDO", ID: "df9b44b7-c38e-4652-834f-6cfc3cf9a70e", PlayerID: 0, Reference: "9d36cd19-0694-4e79-ad48-e5678663030c", Team: "NONE", Timestamp: 1777841681973, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "868c7f4f-0719-4c98-ae0f-dde187669a2e", PlayerID: 0, Reference: "", Team: "AWAY", Timestamp: 1777841682760, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "b4ffa7cb-5ef4-4e3a-98f1-0b47a08df2ba", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777841683899, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "UNDO", ID: "6626d027-c612-421d-a24a-3eefca606b51", PlayerID: 0, Reference: "b4ffa7cb-5ef4-4e3a-98f1-0b47a08df2ba", Team: "NONE", Timestamp: 1777841948587, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "UNDO", ID: "4c7cd037-3ba8-4c73-9506-1e52f006ae28", PlayerID: 0, Reference: "868c7f4f-0719-4c98-ae0f-dde187669a2e", Team: "NONE", Timestamp: 1777841948754, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "d705d142-fcdb-4e73-b96b-bc6248356a60", PlayerID: 0, Reference: "", Team: "AWAY", Timestamp: 1777841949522, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "dca443b0-2c96-4f53-a141-c9f3906e5f47", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777841949930, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "UNDO", ID: "678d64fe-116e-43f4-b348-68d060ab7a46", PlayerID: 0, Reference: "dca443b0-2c96-4f53-a141-c9f3906e5f47", Team: "NONE", Timestamp: 1777842265894, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "UNDO", ID: "a22702f7-32c9-4de0-98fd-26f3d1edf23e", PlayerID: 0, Reference: "d705d142-fcdb-4e73-b96b-bc6248356a60", Team: "NONE", Timestamp: 1777842266734, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "538d7342-da61-4b87-8468-7d87a77a8400", PlayerID: 0, Reference: "", Team: "AWAY", Timestamp: 1777842267614, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "023fefae-d3e3-4a8c-819e-1459e667fd13", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777842268681, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "UNDO", ID: "a101e2e9-26f4-426c-a11b-ba6e1588772f", PlayerID: 0, Reference: "023fefae-d3e3-4a8c-819e-1459e667fd13", Team: "NONE", Timestamp: 1777843077132, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "UNDO", ID: "b34f34aa-4dfe-4394-9063-71f2ed265c1d", PlayerID: 0, Reference: "538d7342-da61-4b87-8468-7d87a77a8400", Team: "NONE", Timestamp: 1777843077761, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "210a994c-ff30-455d-8c24-c5aa4830774e", PlayerID: 0, Reference: "", Team: "AWAY", Timestamp: 1777843079559, Undone: ""},
				{Author: "FFL25Y81t2avn9J1Ak3yq48jgBI3", EventType: "SCORE", ID: "72df54d7-4e51-4b24-9b06-a4de4d267200", PlayerID: 0, Reference: "", Team: "HOME", Timestamp: 1777843080309, Undone: ""},
			},
			now:      time.UnixMilli(1777843080309 + (6 * 60 * 1000)),
			expected: nil,
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

func TestBuildTournamentFinalizeUpdates(t *testing.T) {
	updates := buildTournamentFinalizeUpdates()

	if len(updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updates))
	}

	if updates[0].Path != "IsFinalized" {
		t.Fatalf("expected update path IsFinalized, got %s", updates[0].Path)
	}

	value, ok := updates[0].Value.(bool)
	if !ok {
		t.Fatalf("expected bool value, got %T", updates[0].Value)
	}
	if !value {
		t.Fatalf("expected IsFinalized value to be true")
	}
}
