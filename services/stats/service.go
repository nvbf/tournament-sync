package stats

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go/v4"
	"github.com/gin-gonic/gin"
	timehelper "github.com/nvbf/tournament-sync/pkg/timeHelper"
	profixio "github.com/nvbf/tournament-sync/repos/profixio"
)

type StatsService struct {
	firestoreClient *firestore.Client
	firebaseApp     *firebase.App
	profixioService *profixio.Service
}

func NewStatsService(firestoreClient *firestore.Client, firebaseApp *firebase.App) *StatsService {
	return &StatsService{
		firestoreClient: firestoreClient,
		firebaseApp:     firebaseApp,
	}
}

func (s *StatsService) GetStats(c *gin.Context) ([]*TournamentStats, error) {
	var tournaments []*TournamentStats

	docs, err := s.firestoreClient.Collection("Tournaments").
		Where("NumberOfScoreboards", ">", 0).
		Documents(c).
		GetAll()

	if err != nil {
		log.Printf("Failed to write tournament to Firestore: %v\n", err)
		return nil, err
	}

	fmt.Printf("Length of list %d\n", len(docs))

	for _, doc := range docs {
		tournament, err := docToTournamentStats(doc)
		if err != nil {
			return nil, err
		}

		tournaments = append(tournaments, tournament)
	}

	sort.Slice(tournaments, func(i, j int) bool {
		return tournaments[i].StartDate < tournaments[j].StartDate
	})

	total := 0
	totalScoreboards := 0
	for i, v := range tournaments {
		fmt.Printf("#%d\t%s\t-\t%s\tscoreboards: %d / %d\n", i, v.StartDate, v.Name, v.NumberOfScoreboards, v.NumberOfMatches)
		total = v.NumberOfMatches + total
		totalScoreboards = v.NumberOfScoreboards + totalScoreboards
	}
	fmt.Printf("Total scoreboards: %d / %d\n", totalScoreboards, total)

	// Send the processed tournament to the channel
	return tournaments, nil
}

func (s *StatsService) UpdateStats(c *gin.Context) error {

	var tournaments []*TournamentStats

	docs, err := s.firestoreClient.Collection("Tournaments").
		Where("EndDate", "<", timehelper.GetTodaysDateString()).
		Where("StatsWritten", "==", false).
		Documents(c).
		GetAll()

	if err != nil {
		log.Printf("Failed to write tournament to Firestore: %v\n", err)
		return err
	}

	fmt.Printf("Length of list %d\n", len(docs))

	for _, doc := range docs {
		tournament, err := docToTournamentStats(doc)
		if err != nil {
			return err
		}
		if !tournament.StatsWritten {
			tournaments = append(tournaments, tournament)
		}
	}

	for _, v := range tournaments {

		if strings.TrimSpace(v.Slug) == "" {
			continue
		}

		matchDocs, err := s.firestoreClient.Collection("Tournaments").Doc(v.Slug).Collection("Matches").Documents(c).GetAll()
		if err != nil {
			log.Printf("Failed to write tournament to Firestore: %v\n", err)
			return err
		}

		if len(matchDocs) == 0 {
			continue
		}

		scoreboards := 0
		totalMatches := 0
		for _, matchDoc := range matchDocs {
			var match Match
			if err := matchDoc.DataTo(&match); err != nil {
				// If this fails, we have an inconsistency error as we control both the data written to
				// Firestore and the shape of our `fsIntegration` struct.
				return fmt.Errorf(
					"consistency error. Converting %+v to internal integration struct failed: %w",
					matchDoc,
					err,
				)
			}
			totalMatches++
			if match.ScoreboardId != "" {
				scoreboards++
			}
		}

		updates := []firestore.Update{
			{Path: "NumberOfScoreboards", Value: scoreboards},
			{Path: "NumberOfMatches", Value: totalMatches},
			{Path: "StatsWritten", Value: true},
		}

		_, err = s.firestoreClient.Collection("Tournaments").Doc(v.Slug).Update(c, updates)
		if err != nil {
			log.Printf("Failed to update tournament to Firestore: %v\n", err)
			return err
		}
	}

	return nil
}

func docToTournamentStats(doc *firestore.DocumentSnapshot) (*TournamentStats, error) {
	var tournament TournamentStats
	if err := doc.DataTo(&tournament); err != nil {
		// If this fails, we have an inconsistency error as we control both the data written to
		// Firestore and the shape of our `fsIntegration` struct.
		return nil, fmt.Errorf(
			"consistency error. Converting %+v to internal integration struct failed: %w",
			doc,
			err,
		)
	}

	return &tournament, nil
}

type stats struct {
	NumberOfScoreboards int  `firestore:"NumberOfScoreboards"`
	NumberOfMatches     int  `firestore:"NumberOfMatches"`
	StatsWritten        bool `firestore:"StatsWritten"`
}
