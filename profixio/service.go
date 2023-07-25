package profixio

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"

	"cloud.google.com/go/firestore"
	"golang.org/x/xerrors"
)

// Service represents the migration status of a single service.
type Service struct {
	Client *firestore.Client
}

// NewService creates a new empty service.
func NewService(client *firestore.Client) *Service {
	return &Service{
		Client: client,
	}
}

func (s Service) FetchTournaments(ctx context.Context, pageId int) {

	// Make the API call to fetch the tournaments
	apiURL := fmt.Sprintf("https://www.profixio.com/app/api/organisations/NVBF.NO.VB/tournaments?limit=5&sportId=SVB&page=%d", pageId)

	// Create an HTTP client
	httpClient := &http.Client{}

	// Create an HTTP request
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		log.Fatalf("Failed to create HTTP request: %v", err)
	}

	// Add the API key as a header
	apiKey := os.Getenv("PROFIXIO_KEY")
	req.Header.Set("x-api-secret", apiKey)

	// Send the HTTP request
	response, err := httpClient.Do(req)
	if err != nil {
		log.Fatalf("API request failed: %v", err)
	}
	defer response.Body.Close()

	// Parse the API response into the APIResponse struct
	var apiResponse TournamentResponse
	err = json.NewDecoder(response.Body).Decode(&apiResponse)
	if err != nil {
		log.Fatalf("Failed to parse API response: %v", err)
	}

	// Create a wait group to wait for all goroutines to finish
	var wg sync.WaitGroup

	// Create a channel to receive tournament data from goroutines
	tournamentCh := make(chan Tournament)

	// Start concurrent goroutines to process tournaments
	for _, tournament := range apiResponse.Data {
		wg.Add(1)
		go s.processTournament(ctx, tournament, tournamentCh, &wg)
	}

	// Close the channel when all goroutines finish
	go func() {
		wg.Wait()
		close(tournamentCh)
	}()

	// Iterate over the channel to receive tournament data
	for tournament := range tournamentCh {
		// Do something with the tournament data
		log.Printf("Processed tournament: %+v\n", tournament)
	}

	lastPage := apiResponse.Meta.LastPage
	if err != nil {
		log.Println("Cloud not parse request")
		return
	}

	var wg2 sync.WaitGroup

	for i := 2; i <= lastPage; i++ {
		wg2.Add(1)
		go s.fetchTournamentPage(ctx, i, &wg2)
	}
	wg2.Wait()

	log.Println("All tournaments processed")
}

func (s Service) fetchTournamentPage(ctx context.Context, pageId int, wgx *sync.WaitGroup) {
	defer wgx.Done()

	// Make the API call to fetch the tournaments
	apiURL := fmt.Sprintf("https://www.profixio.com/app/api/organisations/NVBF.NO.VB/tournaments?limit=5&sportId=SVB&page=%d", pageId)

	// Create an HTTP client
	httpClient := &http.Client{}

	// Create an HTTP request
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		log.Fatalf("Failed to create HTTP request: %v", err)
	}

	// Add the API key as a header
	apiKey := os.Getenv("PROFIXIO_KEY")
	req.Header.Set("x-api-secret", apiKey)

	// Send the HTTP request
	response, err := httpClient.Do(req)
	if err != nil {
		log.Fatalf("API request failed: %v", err)
	}
	defer response.Body.Close()

	// Parse the API response into the APIResponse struct
	var apiResponse TournamentResponse
	err = json.NewDecoder(response.Body).Decode(&apiResponse)
	if err != nil {
		log.Fatalf("Failed to parse API response: %v", err)
	}

	// Create a wait group to wait for all goroutines to finish
	var wg sync.WaitGroup

	// Create a channel to receive tournament data from goroutines
	tournamentCh := make(chan Tournament)

	// Start concurrent goroutines to process tournaments
	for _, tournament := range apiResponse.Data {
		wg.Add(1)
		go s.processTournament(ctx, tournament, tournamentCh, &wg)
	}

	// Close the channel when all goroutines finish
	go func() {
		wg.Wait()
		close(tournamentCh)
	}()

	log.Printf("Page done: %d\n", pageId)

}

func (s Service) processTournament(ctx context.Context, tournament Tournament, tournamentCh chan<- Tournament, wg *sync.WaitGroup) {
	defer wg.Done()

	// Get a document
	docRef := s.Client.Collection("Tournaments").Doc(*tournament.Slug)

	doc, _ := docRef.Get(ctx)

	if doc.Exists() {

		updates := createTournamentUpdates(&tournament)

		// Write the tournament to Firestore
		_, err := s.Client.Collection("Tournaments").Doc(*tournament.Slug).Update(ctx, updates)
		if err != nil {
			log.Printf("Failed to update tournament to Firestore: %v\n", err)
			return
		}
	} else {
		_, err := s.Client.Collection("Tournaments").Doc(*tournament.Slug).Set(ctx, tournament)
		if err != nil {
			log.Printf("Failed to set tournament to Firestore: %v\n", err)
			return
		}
	}

	// Send the processed tournament to the channel
	tournamentCh <- tournament
	log.Printf("processTournament done")
}

func (s Service) FetchMatches(ctx context.Context, pageId int, slug string, lastSync string, timeNow string) {

	tournamentID, err := s.getTournamentId(ctx, slug)

	if err != nil {
		log.Printf("Did not get tournamentId from firestore")
		return
	}

	// Make the API call to fetch the tournaments
	apiURL := fmt.Sprintf("https://www.profixio.com/app/api/tournaments/%d/matches?limit=150&page=%d", tournamentID, pageId)
	if lastSync != "" {
		apiURL = fmt.Sprintf("https://www.profixio.com/app/api/tournaments/%d/matches?limit=150&page=%d&updated=%s", tournamentID, pageId, lastSync)
	}

	// Create an HTTP client
	httpClient := &http.Client{}

	// Create an HTTP request
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		log.Fatalf("Failed to create HTTP request: %v", err)
	}

	// Add the API key as a header
	apiKey := os.Getenv("PROFIXIO_KEY")
	req.Header.Set("x-api-secret", apiKey)

	// Send the HTTP request
	response, err := httpClient.Do(req)
	if err != nil {
		log.Fatalf("API request failed: %v", err)
	}
	defer response.Body.Close()

	// Parse the API response into the APIResponse struct
	var apiResponse MatchResponse
	err = json.NewDecoder(response.Body).Decode(&apiResponse)
	if err != nil {
		log.Fatalf("Failed to parse API response for tourId %d, slugId %s, page %d: %v", tournamentID, slug, pageId, err)
	}

	// Create a wait group to wait for all goroutines to finish
	var wg sync.WaitGroup

	// Create a channel to receive tournament data from goroutines
	matchCh := make(chan Match)

	// Start concurrent goroutines to process tournaments
	for _, match := range apiResponse.Data {
		wg.Add(1)

		go s.processMatches(ctx, slug, match, matchCh, &wg)
	}

	// Close the channel when all goroutines finish
	go func() {
		wg.Wait()
		close(matchCh)
	}()

	// Iterate over the channel to receive tournament data
	for match := range matchCh {
		// Do something with the tournament data
		log.Printf("Processed match: %s\n", *match.Number)
	}

	lastPage := apiResponse.Meta.LastPage

	var wg2 sync.WaitGroup

	for i := 2; i <= lastPage; i++ {
		wg2.Add(1)
		go s.fetchMatchesPage(ctx, i, slug, lastSync, timeNow, &wg2)
	}
	wg2.Wait()

	s.setLastSynced(ctx, slug, timeNow)
	log.Println("All matches processed")
}

func (s Service) fetchMatchesPage(ctx context.Context, pageId int, slug string, lastSync string, timeNow string, wgx *sync.WaitGroup) {
	defer wgx.Done()

	tournamentID, err := s.getTournamentId(ctx, slug)

	if err != nil {
		log.Printf("Did not get tournamentId from firestore")
		return
	}

	// Make the API call to fetch the tournaments
	apiURL := fmt.Sprintf("https://www.profixio.com/app/api/tournaments/%d/matches?limit=150&page=%d", tournamentID, pageId)
	if lastSync != "" {
		apiURL = fmt.Sprintf("https://www.profixio.com/app/api/tournaments/%d/matches?limit=150&page=%d&updated=%s", tournamentID, pageId, lastSync)
	}

	// Create an HTTP client
	httpClient := &http.Client{}

	// Create an HTTP request
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		log.Fatalf("Failed to create HTTP request: %v", err)
	}

	// Add the API key as a header
	apiKey := os.Getenv("PROFIXIO_KEY")
	req.Header.Set("x-api-secret", apiKey)

	// Send the HTTP request
	response, err := httpClient.Do(req)
	if err != nil {
		log.Fatalf("API request failed: %v", err)
	}
	defer response.Body.Close()

	// Parse the API response into the APIResponse struct
	var apiResponse MatchResponse
	err = json.NewDecoder(response.Body).Decode(&apiResponse)
	if err != nil {
		log.Fatalf("Failed to parse API response for tourId %d, slugId %s, page %d: %v", tournamentID, slug, pageId, err)
	}

	// Create a wait group to wait for all goroutines to finish
	var wg sync.WaitGroup

	// Create a channel to receive tournament data from goroutines
	matchCh := make(chan Match)

	// Start concurrent goroutines to process tournaments
	for _, match := range apiResponse.Data {
		wg.Add(1)

		go s.processMatches(ctx, slug, match, matchCh, &wg)
	}

	// Close the channel when all goroutines finish
	go func() {
		wg.Wait()
		close(matchCh)
	}()

	// Iterate over the channel to receive tournament data
	for match := range matchCh {
		// Do something with the tournament data
		log.Printf("Processed match: %s\n", *match.Number)
	}
}

func (s Service) processMatches(ctx context.Context, slug string, match Match, matchCh chan<- Match, wg *sync.WaitGroup) {
	defer wg.Done()
	// Get a document
	docRef := s.Client.Collection("Tournaments").Doc(slug).Collection("Matches").Doc(*match.Number)

	doc, _ := docRef.Get(ctx)

	if doc.Exists() {
		updates := createMatchUpdates(&match)

		// Update the match in Firestore
		_, err := s.Client.Collection("Tournaments").Doc(slug).Collection("Matches").Doc(*match.Number).Update(ctx, updates)
		if err != nil {
			log.Printf("Failed to update match in Firestore: %v\n", err)
			return
		}
	} else {
		// Write the match to Firestore
		_, err := s.Client.Collection("Tournaments").Doc(slug).Collection("Matches").Doc(*match.Number).Set(ctx, match)
		if err != nil {
			log.Printf("Failed to write match to Firestore: %v\n", err)
			return
		}
	}

	// Send the processed tournament to the channel
	matchCh <- match
}

func (s Service) getTournamentId(ctx context.Context, slug string) (int, error) {
	var tournament Tournament

	// Write the tournament to Firestore
	doc, err := s.Client.Collection("Tournaments").Doc(slug).Get(ctx)
	if err != nil {
		log.Printf("Failed to write tournament to Firestore: %v\n", err)
		return -1, err
	}

	if err := doc.DataTo(&tournament); err != nil {
		// If this fails, we have an inconsistency error as we control both the data written to
		// Firestore and the shape of our `fsIntegration` struct.
		log.Printf("Could not parse tournament %v", err)
		return -1, xerrors.Errorf(
			"consistency error. Converting %+v to internal integration struct failed: %w",
			doc,
			err,
		)
	}
	// Send the processed tournament to the channel
	return *tournament.ID, nil
}

func (s Service) GetLastSynced(ctx context.Context, slug string) string {
	// Write the tournament to Firestore
	doc, err := s.Client.Collection("Tournaments").Doc(slug).Get(ctx)
	if err != nil {
		log.Printf("Failed to write tournament to Firestore: %v\n", err)
		return ""
	}

	data := doc.Data()
	fieldValue, ok := data["LastSynced"]
	if !ok {
		log.Printf("Field does not exist in the document.")
	}

	fieldValueStr, ok := fieldValue.(string)
	if !ok {
		log.Printf("Failed to convert field value to string.")
	}

	// Send the processed tournament to the channel
	return fieldValueStr
}

func (s Service) GetLastRequest(ctx context.Context, slug string) string {
	// Write the tournament to Firestore
	doc, err := s.Client.Collection("Tournaments").Doc(slug).Get(ctx)
	if err != nil {
		log.Printf("Failed to write tournament to Firestore: %v\n", err)
		return ""
	}

	data := doc.Data()
	fieldValue, ok := data["LastRequest"]
	if !ok {
		log.Printf("Field does not exist in the document.")
	}

	fieldValueStr, ok := fieldValue.(string)
	if !ok {
		log.Printf("Failed to convert field value to string.")
	}

	// Send the processed tournament to the channel
	return fieldValueStr
}

func (s Service) setLastSynced(ctx context.Context, slug string, lastSynced string) error {
	// Write the tournament to Firestore
	_, err := s.Client.Collection("Tournaments").Doc(slug).Update(ctx, []firestore.Update{
		{
			Path:  "LastSynced",
			Value: lastSynced,
		},
	})
	if err != nil {
		// Handle any errors in an appropriate way, such as returning them.
		log.Printf("An error has occurred: %v", err)
	}
	// Send the processed tournament to the channel
	return nil
}

func (s Service) SetLastRequest(ctx context.Context, slug string, lastRequest string) error {
	// Write the tournament to Firestore
	_, err := s.Client.Collection("Tournaments").Doc(slug).Update(ctx, []firestore.Update{
		{
			Path:  "LastRequest",
			Value: lastRequest,
		},
	})
	if err != nil {
		// Handle any errors in an appropriate way, such as returning them.
		log.Printf("An error has occurred: %v", err)
	}
	// Send the processed tournament to the channel
	return nil
}

func createTournamentUpdates(tournament *Tournament) []firestore.Update {
	var updates []firestore.Update

	if tournament.ID != nil {
		updates = append(updates, firestore.Update{Path: "Id", Value: *tournament.ID})
	}
	if tournament.Name != nil {
		updates = append(updates, firestore.Update{Path: "Name", Value: *tournament.Name})
	}
	if tournament.Slug != nil {
		updates = append(updates, firestore.Update{Path: "Slug", Value: *tournament.Slug})
	}
	if tournament.StartDate != nil {
		updates = append(updates, firestore.Update{Path: "StartDate", Value: *tournament.StartDate})
	}
	if tournament.EndDate != nil {
		updates = append(updates, firestore.Update{Path: "EndDate", Value: *tournament.EndDate})
	}

	return updates
}

func createMatchUpdates(match *Match) []firestore.Update {
	var updates []firestore.Update

	if match.ID != nil {
		updates = append(updates, firestore.Update{Path: "Id", Value: *match.ID})
	}
	if match.Txid != nil {
		updates = append(updates, firestore.Update{Path: "Txid", Value: *match.Txid})
	}
	if match.TournamentID != nil {
		updates = append(updates, firestore.Update{Path: "TournamentId", Value: *match.TournamentID})
	}
	if match.GameRound != nil {
		updates = append(updates, firestore.Update{Path: "GameRound", Value: *match.GameRound})
	}
	if match.PlayoffLevel != nil {
		updates = append(updates, firestore.Update{Path: "PlayoffLevel", Value: *match.PlayoffLevel})
	}
	if match.Number != nil {
		updates = append(updates, firestore.Update{Path: "Number", Value: *match.Number})
	}
	if match.Name != nil {
		updates = append(updates, firestore.Update{Path: "Name", Value: *match.Name})
	}
	if match.Date != nil {
		updates = append(updates, firestore.Update{Path: "Date", Value: *match.Date})
	}
	if match.Time != nil {
		updates = append(updates, firestore.Update{Path: "Time", Value: *match.Time})
	}
	if match.WinnerTeam != nil {
		updates = append(updates, firestore.Update{Path: "WinnerTeam", Value: *match.WinnerTeam})
	}
	if match.SettResultsFormatted != nil {
		updates = append(updates, firestore.Update{Path: "SettResultsFormatted", Value: *match.SettResultsFormatted})
	}
	if match.MatchDataUpdated != nil {
		updates = append(updates, firestore.Update{Path: "MatchDataUpdated", Value: *match.MatchDataUpdated})
	}
	if match.ResultsUpdated != nil {
		updates = append(updates, firestore.Update{Path: "ResultsUpdated", Value: *match.ResultsUpdated})
	}
	if match.HasWinner != nil {
		updates = append(updates, firestore.Update{Path: "HasWinner", Value: *match.HasWinner})
	}
	if match.IsHidden != nil {
		updates = append(updates, firestore.Update{Path: "IsHidden", Value: *match.IsHidden})
	}
	if match.IsGroupPlay != nil {
		updates = append(updates, firestore.Update{Path: "IsGroupPlay", Value: *match.IsGroupPlay})
	}
	if match.IsPlayoff != nil {
		updates = append(updates, firestore.Update{Path: "IsPlayoff", Value: *match.IsPlayoff})
	}
	if match.IncludedInTableCalculation != nil {
		updates = append(updates, firestore.Update{Path: "IncludedInTableCalculation", Value: *match.IncludedInTableCalculation})
	}
	if match.HomeTeam != nil {
		updates = append(updates, firestore.Update{Path: "HomeTeam", Value: match.HomeTeam})
	}
	if match.AwayTeam != nil {
		updates = append(updates, firestore.Update{Path: "AwayTeam", Value: match.AwayTeam})
	}
	if match.Field != nil {
		updates = append(updates, firestore.Update{Path: "Field", Value: match.Field})
	}
	if match.MatchGroup != nil {
		updates = append(updates, firestore.Update{Path: "MatchGroup", Value: match.MatchGroup})
	}
	if match.MatchCategory != nil {
		updates = append(updates, firestore.Update{Path: "MatchCategory", Value: match.MatchCategory})
	}
	if match.Sets != nil {
		updates = append(updates, firestore.Update{Path: "Sets", Value: match.Sets})
	}
	if match.RefereesTX != nil {
		updates = append(updates, firestore.Update{Path: "RefereesTX", Value: match.RefereesTX})
	}

	return updates
}
