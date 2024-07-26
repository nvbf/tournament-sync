package matches

import (
	"testing"

	profixio "github.com/nvbf/tournament-sync/repos/profixio"
)

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
