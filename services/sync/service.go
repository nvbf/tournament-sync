package sync

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go/v4"
	"github.com/gin-gonic/gin"
	profixio "github.com/nvbf/tournament-sync/repos/profixio"
	"github.com/xorcare/pointer"
)

type SyncService struct {
	firestoreClient *firestore.Client
	firebaseApp     *firebase.App
	profixioService *profixio.Service
}

func NewSyncService(firestoreClient *firestore.Client, firebaseApp *firebase.App, profixioService *profixio.Service) *SyncService {
	return &SyncService{
		firestoreClient: firestoreClient,
		firebaseApp:     firebaseApp,
		profixioService: profixioService,
	}
}

func (s *SyncService) FetchTournaments(c *gin.Context) error {
	ctx := context.Background()
	go s.profixioService.FetchTournaments(ctx, 1)

	c.JSON(http.StatusOK, gin.H{
		"message": "Async function started",
	})
	return nil
}

func (s *SyncService) SyncTournamentMatches(c *gin.Context, slug string) error {
	layout := "2006-01-02 15:04:05"

	if s.profixioService.IsCustomTournament(c, slug) {
		log.Printf("Don't sync custom tournament\n")
		return nil
	}

	t := time.Now()
	t_m := time.Now().Add(-10 * time.Minute)
	now := t.Format(layout)
	now_m := t_m.Format(layout)

	ctx := context.Background()
	lastSync := s.profixioService.GetLastSynced(ctx, slug)
	lastReq := s.profixioService.GetLastRequest(ctx, slug)
	if lastReq == "" {
		lastReq = layout
	}
	lastRequestTime, err := time.Parse(layout, lastReq)
	if err != nil {
		fmt.Println(err)
	}
	newTime := t.Add(0 * time.Hour)
	diff := newTime.Sub(lastRequestTime)
	if diff < 0*time.Second {
		newTime = t.Add(2 * time.Hour)
		diff = newTime.Sub(lastRequestTime)
	}

	log.Printf("Since last req: %s\n", diff)

	if diff < 30*time.Second {
		c.JSON(http.StatusOK, gin.H{
			"message": fmt.Sprintf("Seconds since last req: %s", diff),
		})
	} else {
		s.profixioService.SetLastRequest(ctx, slug, now)
		go s.profixioService.FetchMatches(ctx, 1, slug, lastSync, now_m)

		c.JSON(http.StatusOK, gin.H{
			"message": fmt.Sprintf("Async function started sync from lastSync: %s", lastSync),
		})
	}
	return nil
}

func (s *SyncService) UpdateCustomTournament(c *gin.Context, slug string, tournament profixio.CustomTournament) error {
	go s.profixioService.ProcessCustomTournament(c, slug, tournament)

	return nil
}

func (s *SyncService) CreateIfNoExisting(c *gin.Context, slug string) error {

	tournament := profixio.Tournament{
		ID:        pointer.Int(2405),
		Name:      pointer.String("Nevza Oddanesand 2024"),
		Slug:      pointer.String(slug),
		StartDate: pointer.String("2024-06-18"),
		EndDate:   pointer.String("2024-06-20"),
		Type:      pointer.String("Custom"),
	}

	s.profixioService.SetCustomTournament(c, tournament)
	return nil
}

func (s *SyncService) GetStats(c *gin.Context) error {

	var tournaments []*Tournament
	var stats []*Stats

	docs, err := s.firestoreClient.Collection("Tournaments").
		// Where("Matches", "!=", nil).
		Documents(c).
		GetAll()

	if err != nil {
		log.Printf("Failed to write tournament to Firestore: %v\n", err)
		return err
	}

	fmt.Printf("Length of list %d\n", len(docs))

	for _, doc := range docs {
		tournament, err := docToTournament(doc)
		if err != nil {
			return err
		}

		tournaments = append(tournaments, tournament)
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
		if scoreboards < totalMatches/10 || scoreboards == 0 {
			continue
		}
		fmt.Printf("%s with scoreBoards %d/%d\n", v.Name, scoreboards, totalMatches)
		stats = append(stats, &Stats{
			StartDate:    v.StartDate,
			Name:         v.Name,
			Scoreboards:  scoreboards,
			TotalMatches: totalMatches,
		})

	}

	sort.Slice(stats, func(i, j int) bool {
		return stats[i].StartDate < stats[j].StartDate
	})

	total := 0
	totalScoreboards := 0
	for i, v := range stats {
		fmt.Printf("#%d\t%s\t-\t%s\tscoreboards: %d / %d\n", i, v.StartDate, v.Name, v.Scoreboards, v.TotalMatches)
		total = v.TotalMatches + total
		totalScoreboards = v.Scoreboards + totalScoreboards
	}
	fmt.Printf("Total scoreboards: %d / %d\n", totalScoreboards, total)

	// Send the processed tournament to the channel
	return nil
}

func docToTournament(doc *firestore.DocumentSnapshot) (*Tournament, error) {
	var tournament Tournament
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

type Tournament struct {
	Name      string  `firestore:"Name"`
	Type      string  `firestore:"Type"`
	Slug      string  `firestore:"Slug"`
	StartDate string  `firestore:"StartDate"`
	EndDate   string  `firestore:"EndDate"`
	Matches   []Match `firestore:"Matches"`
}

type Match struct {
	ScoreboardId string `firestore:"ScoreboardId"`
	Number       string `firestore:"Number"`
}

type Stats struct {
	StartDate    string
	Name         string
	Scoreboards  int
	TotalMatches int
}
