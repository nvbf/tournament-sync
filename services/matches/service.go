package matches

import (
	"fmt"
	"log"
	"sort"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go/v4"
	auth "firebase.google.com/go/v4/auth"
	"google.golang.org/api/iterator"

	"github.com/gin-gonic/gin"

	profixio "github.com/nvbf/tournament-sync/repos/profixio"
)

type MatchesService struct {
	firestoreClient *firestore.Client
	firebaseApp     *firebase.App
	profixioService *profixio.Service
}

func NewMatchesService(firestoreClient *firestore.Client, firebaseApp *firebase.App, profixioService *profixio.Service) *MatchesService {
	return &MatchesService{
		firestoreClient: firestoreClient,
		firebaseApp:     firebaseApp,
		profixioService: profixioService,
	}
}

func (s *MatchesService) ReportResult(c *gin.Context, matchID string) error {
	token := c.MustGet("token").(*auth.Token)

	_, err := s.firestoreClient.Collection("Matches").Doc(matchID).Update(c,
		[]firestore.Update{
			{Path: "AutoReport", Value: false},
		},
	)
	if err != nil {
		log.Printf("Failed to update match in Firestore: %v\n", err)
		return err
	}

	iter := s.firestoreClient.Collection("Matches").Doc(matchID).Collection("events").Documents(c)
	defer iter.Stop()

	authorMissmatches := 0

	var events []Event
	for {
		doc, err := iter.Next()
		if err != nil {
			if err == iterator.Done {
				break
			}
			log.Printf("Failed to get document: %v\n", err)
			return nil
		}

		var event Event
		if err := doc.DataTo(&event); err != nil {
			log.Printf("Failed to decode document: %v\n", err)
			return nil
		}
		if event.Author != token.UID {
			fmt.Printf("For event: %s - %s: Not the same author: %s vs. %s\n", event.EventType, event.ID, token.UID, event.Author)
			authorMissmatches++
		}
		events = append(events, event)
	}

	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp < events[j].Timestamp
	})

	matchResult := processEvents(events)
	doc, err := s.firestoreClient.Collection("Matches").Doc(matchID).Get(c)
	if err != nil {
		log.Printf("Failed to get tournament from Firestore: %v\n", err)
		return err
	}

	data := doc.Data()
	fieldValue, ok := data["matchId"]
	if !ok {
		log.Printf("Field 'matchId' does not exist in the document.")
	}

	matchNumber, ok := fieldValue.(string)
	if !ok {
		log.Printf("Failed to convert field value 'matchId' to string.")
		return nil
	}
	fieldValue, ok = data["tournamentId"]
	if !ok {
		log.Printf("Field 'tournamentId' does not exist in the document.")
	}

	slug, ok := fieldValue.(string)
	if !ok {
		log.Printf("Failed to convert field value 'tournamentId' to string.")
		return nil
	}

	doc, err = s.firestoreClient.Collection("TournamentSecrets").Doc(slug).Get(c)
	if err != nil {
		log.Printf("Failed to get tournament to Firestore: %v\n", err)
		return err
	}

	data = doc.Data()
	fieldValue, ok = data["ID"]
	if !ok {
		log.Printf("Field 'ID' does not exist in the document. %v", fieldValue)
	}

	tournamentSecretID, ok := fieldValue.(int64)
	if !ok {
		log.Printf("Failed to convert field value 'ID' to int from slug %s.  %v", fieldValue, slug)
		return nil
	}

	doc, err = s.firestoreClient.Collection("Tournaments").Doc(slug).Collection("Matches").Doc(matchNumber).Get(c)
	if err != nil {
		log.Printf("Failed to get tournament match from Firestore: %v\n", err)
		return err
	}

	data = doc.Data()
	fieldValue, ok = data["ID"]
	if !ok {
		log.Printf("Field 'ID' does not exist in the document. %v", fieldValue)
	}

	matchSecretID, ok := fieldValue.(int64)
	if !ok {
		log.Printf("Failed to convert field value 'ID' to int from slug %s.  %v", fieldValue, slug)
		return nil
	}
	tournamentSecretIDString := fmt.Sprint(tournamentSecretID)
	matchSecretIDString := fmt.Sprint(matchSecretID)

	if !validateMatchResult(matchResult) {
		_, err = s.firestoreClient.Collection("Matches").Doc(matchID).Update(c,
			[]firestore.Update{
				{Path: "AuthorMissmatches", Value: authorMissmatches},
				{Path: "Invalid", Value: true},
			},
		)
		if err != nil {
			log.Printf("Failed to update match in Firestore: %v\n", err)
			return err
		}

		_, err = s.firestoreClient.Collection("Tournaments").Doc(slug).Collection("Matches").Doc(matchNumber).Update(c,
			[]firestore.Update{
				{Path: "MatchResultValid", Value: false},
			},
		)
		if err != nil {
			log.Printf("Failed to update match in Firestore: %v\n", err)
			return err
		}
		return nil
	}

	err = s.profixioService.PostResult(c, matchSecretIDString, tournamentSecretIDString, matchResult)
	if err != nil {
		log.Printf("Failed to report to profixio: %v\n", err)
		return err
	}

	_, err = s.firestoreClient.Collection("Matches").Doc(matchID).Update(c,
		[]firestore.Update{
			{Path: "AutoReport", Value: true},
			{Path: "AuthorMissmatches", Value: authorMissmatches},
		},
	)
	if err != nil {
		log.Printf("Failed to update match in Firestore: %v\n", err)
		return err
	}
	_, err = s.firestoreClient.Collection("Tournaments").Doc(slug).Collection("Matches").Doc(matchNumber).Update(c,
		[]firestore.Update{
			{Path: "MatchResultValid", Value: true},
		},
	)
	if err != nil {
		log.Printf("Failed to update match in Firestore: %v\n", err)
		return err
	}
	return nil
}

func processEvents(events []Event) profixio.MatchResult {
	var sets []profixio.Result
	currentSet := profixio.Result{}
	homeSetsWon := 0
	awaySetsWon := 0
	undoneEvents := map[string]bool{}

	for _, event := range events {
		if event.EventType == "UNDO" {
			undoneEvents[event.Reference] = true
		}
	}

	for _, event := range events {
		if undoneEvents[event.ID] {
			continue
		}

		switch event.EventType {
		case "SCORE":
			if event.Team == "HOME" {
				currentSet.Home++
			} else if event.Team == "AWAY" {
				currentSet.Away++
			}

		case "SET_FINALIZED":
			if currentSet.Home > currentSet.Away {
				homeSetsWon++
			} else {
				awaySetsWon++
			}
			sets = append(sets, currentSet)
			currentSet = profixio.Result{}

		case "MATCH_FINALIZED":
			if currentSet.Home > currentSet.Away {
				homeSetsWon++
			} else {
				awaySetsWon++
			}
			sets = append(sets, currentSet)
		}
	}

	return profixio.MatchResult{
		Sets: sets,
		Result: profixio.Result{
			Home: homeSetsWon,
			Away: awaySetsWon,
		},
	}
}

// Validates the match results according to beach volleyball rules.
func validateMatchResult(match profixio.MatchResult) bool {
	if len(match.Sets) > 3 || len(match.Sets) < 2 {
		return false // Invalid number of sets
	}

	for i, set := range match.Sets {
		if !isValidSetScore(set, i == 2) {
			return false // Invalid score in one of the sets
		}
	}

	// Check the overall match result consistency
	return isValidMatchResult(match)
}

func isValidSetScore(set profixio.Result, isDecidingSet bool) bool {
	homeAdv := set.Home >= set.Away+2
	awayAdv := set.Away >= set.Home+2
	if isDecidingSet {
		// Deciding set must end at 15 or more, with a 2-point lead
		return (set.Home >= 15 && homeAdv) || (set.Away >= 15 && awayAdv)
	}
	// Regular sets must end at 21 or more, with a 2-point lead
	return (set.Home >= 21 && homeAdv) || (set.Away >= 21 && awayAdv)
}

func isValidMatchResult(match profixio.MatchResult) bool {
	homeWins := 0
	awayWins := 0
	for i, set := range match.Sets {
		if i == 2 && (homeWins == 2 || awayWins == 2) {
			return false // Should not be a third set here. Match is done.
		}
		if set.Home > set.Away {
			homeWins++
		} else if set.Away > set.Home {
			awayWins++
		}
	}

	if homeWins != 2 && awayWins != 2 {
		return false // Match is not done
	}

	// Validate match result against set wins
	return (homeWins == match.Result.Home && awayWins == match.Result.Away)
}
