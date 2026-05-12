package sync

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go/v4"
	"github.com/gin-gonic/gin"
	log "github.com/nvbf/tournament-sync/pkg/cloudlog"
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
	log.Info("fetch tournaments start", log.Fields{"operation": "fetchTournaments"})
	go s.profixioService.FetchTournaments(ctx, 1)

	c.JSON(http.StatusOK, gin.H{
		"message": "Async function started",
	})
	log.Info("fetch tournaments dispatched async job", log.Fields{"operation": "fetchTournaments"})
	return nil
}

func (s *SyncService) SyncTournamentMatches(c *gin.Context, slug string, force bool) error {
	layout := "2006-01-02 15:04:05"
	log.Info("sync tournament matches start", log.Fields{"operation": "syncTournamentMatches", "slug": slug, "force": force})

	if s.profixioService.IsCustomTournament(c, slug) {
		log.Info("sync tournament matches skipped custom tournament", log.Fields{"operation": "syncTournamentMatches", "slug": slug})
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
		log.Warning("sync tournament matches parse last request failed", log.Fields{"operation": "syncTournamentMatches", "slug": slug, "lastRequestRaw": lastReq, "parseError": err.Error()})
		lastRequestTime = t.Add(-24 * time.Hour)
	}
	newTime := t.Add(0 * time.Hour)
	diff := newTime.Sub(lastRequestTime)
	if diff < 0*time.Second {
		newTime = t.Add(2 * time.Hour)
		diff = newTime.Sub(lastRequestTime)
	}

	log.Info("sync tournament matches computed timing", log.Fields{"operation": "syncTournamentMatches", "slug": slug, "lastSync": lastSync, "diff": diff.String()})

	if diff < 30*time.Second {
		c.JSON(http.StatusOK, gin.H{
			"message": fmt.Sprintf("Seconds since last req: %s", diff),
		})
		log.Info("sync tournament matches throttled", log.Fields{"operation": "syncTournamentMatches", "slug": slug, "diff": diff.String()})
	} else {
		s.profixioService.SetLastRequest(ctx, slug, now)
		if force {
			go s.profixioService.FetchMatches(ctx, 1, slug, "", now_m)
			c.JSON(http.StatusOK, gin.H{
				"message": "Async function started forced sync",
			})
			log.Info("sync tournament matches dispatched forced sync", log.Fields{"operation": "syncTournamentMatches", "slug": slug, "windowEnd": now_m})
		} else {
			go s.profixioService.FetchMatches(ctx, 1, slug, lastSync, now_m)
			c.JSON(http.StatusOK, gin.H{
				"message": fmt.Sprintf("Async function started sync from lastSync: %s", lastSync),
			})
			log.Info("sync tournament matches dispatched sync", log.Fields{"operation": "syncTournamentMatches", "slug": slug, "from": lastSync, "to": now_m})
		}
	}
	log.Info("sync tournament matches done", log.Fields{"operation": "syncTournamentMatches", "slug": slug})
	return nil
}

func (s *SyncService) SyncTournamentMatch(c *gin.Context, slug string, matchID string) error {
	log.Info("sync tournament match start", log.Fields{"operation": "syncTournamentMatch", "slug": slug, "matchID": matchID})
	doc, err := s.firestoreClient.Collection("TournamentSecrets").Doc(slug).Get(c)
	if err != nil {
		log.Error("sync tournament match get tournament secret failed", err, log.Fields{"operation": "syncTournamentMatch", "slug": slug, "matchID": matchID})
		return err
	}

	data := doc.Data()
	fieldValue, ok := data["ID"]
	if !ok {
		log.Warning("sync tournament match missing field", log.Fields{"operation": "syncTournamentMatch", "collection": "TournamentSecrets", "field": "ID", "slug": slug, "matchID": matchID})
	}

	tournamentSecretID, ok := fieldValue.(int64)
	if !ok {
		log.Warning("sync tournament match invalid field type", log.Fields{"operation": "syncTournamentMatch", "collection": "TournamentSecrets", "field": "ID", "expected": "int64", "slug": slug, "matchID": matchID, "value": fieldValue})
		return nil
	}

	doc, err = s.firestoreClient.Collection("Tournaments").Doc(slug).Collection("Matches").Doc(matchID).Get(c)
	if err != nil {
		log.Error("sync tournament match get match failed", err, log.Fields{"operation": "syncTournamentMatch", "slug": slug, "matchID": matchID})
		return err
	}

	data = doc.Data()
	fieldValue, ok = data["ID"]
	if !ok {
		log.Warning("sync tournament match missing field", log.Fields{"operation": "syncTournamentMatch", "collection": "Matches", "field": "ID", "slug": slug, "matchID": matchID})
	}

	matchSecretID, ok := fieldValue.(int64)
	if !ok {
		log.Warning("sync tournament match invalid field type", log.Fields{"operation": "syncTournamentMatch", "collection": "Matches", "field": "ID", "expected": "int64", "slug": slug, "matchID": matchID, "value": fieldValue})
		return nil
	}

	s.profixioService.FetchMatch(c, slug, matchID, int(tournamentSecretID), int(matchSecretID))
	log.Info("sync tournament match dispatched fetch", log.Fields{"operation": "syncTournamentMatch", "slug": slug, "matchID": matchID})
	return nil
}

func (s *SyncService) CleanupTournaments(c *gin.Context) error {
	log.Info("cleanup tournaments start", log.Fields{"operation": "cleanupTournaments"})

	var tournaments []*Tournament

	docs, err := s.firestoreClient.Collection("Tournaments").
		Where("EndDate", "<", timehelper.GetTodaysDateString()).
		Documents(c).
		GetAll()

	if err != nil {
		log.Error("cleanup tournaments list tournaments failed", err, log.Fields{"operation": "cleanupTournaments"})
		return err
	}

	log.Info("cleanup tournaments loaded candidates", log.Fields{"operation": "cleanupTournaments", "total": len(docs)})

	for _, doc := range docs {
		tournament, err := docToTournament(doc)
		if err != nil {
			return err
		}
		if !tournament.StatsWritten {
			tournaments = append(tournaments, tournament)
		}
	}

	log.Info("cleanup tournaments with missing stats", log.Fields{"operation": "cleanupTournaments", "count": len(tournaments)})

	for _, v := range tournaments {

		if strings.TrimSpace(v.Slug) == "" {
			continue
		}

		matchDocs, err := s.firestoreClient.Collection("Tournaments").Doc(v.Slug).Collection("Matches").Documents(c).GetAll()
		if err != nil {
			log.Error("cleanup tournaments list matches failed", err, log.Fields{"operation": "cleanupTournaments", "slug": v.Slug})
			return err
		}

		if len(matchDocs) == 0 {
			log.Info("cleanup tournaments deleting empty tournament", log.Fields{"operation": "cleanupTournaments", "slug": v.Slug})
			_, err = s.firestoreClient.Collection("Tournaments").Doc(v.Slug).Delete(c)
			if err != nil {
				log.Error("cleanup tournaments delete tournament failed", err, log.Fields{"operation": "cleanupTournaments", "slug": v.Slug})
				return err
			}
			log.Info("cleanup tournaments deleted tournament", log.Fields{"operation": "cleanupTournaments", "slug": v.Slug})
		}
	}

	var tournamentSecrets []*TournamentSecrets

	docs, err = s.firestoreClient.Collection("TournamentSecrets").
		Documents(c).
		GetAll()

	if err != nil {
		log.Error("cleanup tournaments list tournament secrets failed", err, log.Fields{"operation": "cleanupTournaments"})
		return err
	}

	for _, doc := range docs {
		secrets, err := docToTournamentSecrets(doc)
		if err != nil {
			log.Error("cleanup tournaments parse tournament secret failed", err, log.Fields{"operation": "cleanupTournaments"})
			return err
		}
		docRef, err := s.firestoreClient.Collection("Tournaments").Doc(secrets.Slug).Get(c)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				tournamentSecrets = append(tournamentSecrets, secrets)
			} else {
				log.Error("cleanup tournaments get tournament failed", err, log.Fields{"operation": "cleanupTournaments", "slug": secrets.Slug})
				return err
			}
		}
		if !docRef.Exists() {
			tournamentSecrets = append(tournamentSecrets, secrets)
		}
	}

	log.Info("cleanup tournaments tournament secrets to delete", log.Fields{"operation": "cleanupTournaments", "deleteCount": len(tournamentSecrets), "total": len(docs)})

	for _, secret := range tournamentSecrets {
		_, err = s.firestoreClient.Collection("TournamentSecrets").Doc(secret.Slug).Delete(c)
		if err != nil {
			log.Error("cleanup tournaments delete tournament secret failed", err, log.Fields{"operation": "cleanupTournaments", "slug": secret.Slug})
			return err
		}
		log.Info("cleanup tournaments deleted tournament secret", log.Fields{"operation": "cleanupTournaments", "slug": secret.Slug})
	}

	log.Info("cleanup tournaments done", log.Fields{"operation": "cleanupTournaments"})
	return nil
}

func (s *SyncService) UpdateCustomTournament(c *gin.Context, slug string, tournament profixio.CustomTournament) error {
	log.Info("update custom tournament start", log.Fields{"operation": "updateCustomTournament", "slug": slug})
	go s.profixioService.ProcessCustomTournament(c, slug, tournament)
	log.Info("update custom tournament dispatched async job", log.Fields{"operation": "updateCustomTournament", "slug": slug})

	return nil
}

func (s *SyncService) CreateIfNoExisting(c *gin.Context, slug string) error {
	log.Info("create if no existing start", log.Fields{"operation": "createIfNoExisting", "slug": slug})

	tournament := profixio.Tournament{
		ID:        pointer.Int(2405),
		Name:      pointer.String("Nevza Oddanesand 2024"),
		Slug:      pointer.String(slug),
		StartDate: pointer.String("2024-06-18"),
		EndDate:   pointer.String("2024-06-20"),
		Type:      pointer.String("Custom"),
	}

	s.profixioService.SetCustomTournament(c, tournament)
	log.Info("create if no existing set custom tournament", log.Fields{"operation": "createIfNoExisting", "slug": slug})
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
