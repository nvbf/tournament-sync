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

	iter := s.firestoreClient.Collection("Matches").Doc(matchID).Collection("events").Documents(c)
	defer iter.Stop()

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
			fmt.Printf("Not the same author: %s vs. %s\n", token.UID, event.Author)
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

	err = s.profixioService.PostResult(c, matchSecretIDString, tournamentSecretIDString, matchResult)
	if err != nil {
		log.Printf("Failed to report to profixio: %v\n", err)
		return err
	}
	fmt.Printf("Match Result: %+v\n", matchResult)
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
			undoneEvents[event.Undone] = true
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
			currentSet = profixio.Result{}
		}
	}

	if currentSet.Home > 0 || currentSet.Away > 0 {
		if currentSet.Home > currentSet.Away {
			homeSetsWon++
		} else {
			awaySetsWon++
		}
		sets = append(sets, currentSet)
	}

	return profixio.MatchResult{
		Sets: sets,
		Result: profixio.Result{
			Home: homeSetsWon,
			Away: awaySetsWon,
		},
	}
}
