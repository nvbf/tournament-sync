package sync

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go/v4"
	"github.com/gin-gonic/gin"
	timehelper "github.com/nvbf/tournament-sync/pkg/timeHelper"
	profixio "github.com/nvbf/tournament-sync/repos/profixio"
	"github.com/xorcare/pointer"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

func (s *SyncService) SyncTournamentMatches(c *gin.Context, slug string, force bool) error {
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
	if lastReq == "" || force {
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
		if force {
			go s.profixioService.FetchMatches(ctx, 1, slug, "", now_m)
			c.JSON(http.StatusOK, gin.H{
				"message": "Async function started forced sync",
			})
		} else {
			go s.profixioService.FetchMatches(ctx, 1, slug, lastSync, now_m)
			c.JSON(http.StatusOK, gin.H{
				"message": fmt.Sprintf("Async function started sync from lastSync: %s", lastSync),
			})
		}
	}
	return nil
}

func (s *SyncService) SyncTournamentMatch(c *gin.Context, slug string, matchID string) error {
	doc, err := s.firestoreClient.Collection("TournamentSecrets").Doc(slug).Get(c)
	if err != nil {
		log.Printf("Failed to get tournament to Firestore: %v\n", err)
		return err
	}

	data := doc.Data()
	fieldValue, ok := data["ID"]
	if !ok {
		log.Printf("Field 'ID' does not exist in the document. %v", fieldValue)
	}

	tournamentSecretID, ok := fieldValue.(int64)
	if !ok {
		log.Printf("Failed to convert field value 'ID' to int from slug %s.  %v", fieldValue, slug)
		return nil
	}

	doc, err = s.firestoreClient.Collection("Tournaments").Doc(slug).Collection("Matches").Doc(matchID).Get(c)
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

	s.profixioService.FetchMatch(c, slug, matchID, int(tournamentSecretID), int(matchSecretID))
	return nil
}

func (s *SyncService) CleanupTournaments(c *gin.Context) error {

	var tournaments []*Tournament

	docs, err := s.firestoreClient.Collection("Tournaments").
		Where("EndDate", "<", timehelper.GetTodaysDateString()).
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
		if !tournament.StatsWritten {
			tournaments = append(tournaments, tournament)
		}
	}

	fmt.Printf("Tournaments to be deleted %d\n", len(tournaments))

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
			fmt.Printf("Tournament %s has no matches - deleting \n", v.Slug)
			_, err = s.firestoreClient.Collection("Tournaments").Doc(v.Slug).Delete(c)
			if err != nil {
				log.Printf("Failed to delete tournament in Firestore: %v\n", err)
				return err
			}
			fmt.Printf("Tournament %s deleted \n", v.Slug)
		}
	}

	var tournamentSecrets []*TournamentSecrets

	docs, err = s.firestoreClient.Collection("TournamentSecrets").
		Documents(c).
		GetAll()

	if err != nil {
		log.Printf("Failed to write tournament to Firestore: %v\n", err)
		return err
	}

	for _, doc := range docs {
		secrets, err := docToTournamentSecrets(doc)
		if err != nil {
			log.Printf("Failed to parse tournament secrets: %v\n", err)
			return err
		}
		docRef, err := s.firestoreClient.Collection("Tournaments").Doc(secrets.Slug).Get(c)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				tournamentSecrets = append(tournamentSecrets, secrets)
			} else {
				log.Printf("Failed to get tournament from Firestore: %v\n", err)
				return err
			}
		}
		if !docRef.Exists() {
			tournamentSecrets = append(tournamentSecrets, secrets)
		}
	}

	fmt.Printf("Tournaments secrets to be deleted %d from %d\n", len(tournamentSecrets), len(docs))

	for _, secret := range tournamentSecrets {
		_, err = s.firestoreClient.Collection("TournamentSecrets").Doc(secret.Slug).Delete(c)
		if err != nil {
			log.Printf("Failed to delete tournament secret in Firestore: %v\n", err)
			return err
		}
		fmt.Printf("Tournament secrets for %s deleted \n", secret.Slug)
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

func docToTournamentSecrets(doc *firestore.DocumentSnapshot) (*TournamentSecrets, error) {
	var tournamentSecrets TournamentSecrets
	if err := doc.DataTo(&tournamentSecrets); err != nil {
		// If this fails, we have an inconsistency error as we control both the data written to
		// Firestore and the shape of our `fsIntegration` struct.
		return nil, fmt.Errorf(
			"consistency error. Converting %+v to internal integration struct failed: %w",
			doc,
			err,
		)
	}

	return &tournamentSecrets, nil
}

type Tournament struct {
	Name         string  `firestore:"Name"`
	Type         string  `firestore:"Type"`
	Slug         string  `firestore:"Slug"`
	StartDate    string  `firestore:"StartDate"`
	EndDate      string  `firestore:"EndDate"`
	Matches      []Match `firestore:"Matches"`
	StatsWritten bool    `firestore:"StatsWritten"`
}

type TournamentSecrets struct {
	ID     int    `firestore:"ID"`
	Slug   string `firestore:"Slug"`
	Secret string `firestore:"Secret"`
}

type Match struct {
	ScoreboardId string `firestore:"ScoreboardId"`
	Number       string `firestore:"Number"`
}
