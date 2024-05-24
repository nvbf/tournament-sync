package resend

// Define the structure for your JSON payload
type AccessRequest struct {
	Slug         string `json:"slug"`
	TournamentID string `json:"tournamentID"`
	Email        string `json:"email"`
}

// Define the structure for your JSON payload
type ResultRequest struct {
	TournamentID string `json:"tournamentID"`
	MatchID      string `json:"matchID"`
}
