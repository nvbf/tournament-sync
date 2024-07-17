package resend

// Define the structure for your JSON payload
type AccessRequest struct {
	Slug         string `json:"slug"`
	TournamentID int    `json:"tournamentID"`
	Email        string `json:"email"`
}

// Define the structure for your JSON payload
type ResultRequest struct {
	Slug        string `json:"slug"`
	MatchID     string `json:"matchID"`
	MatchNumber string `json:"matchNumber"`
}
