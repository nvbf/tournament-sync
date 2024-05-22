package resend

// Define the structure for your JSON payload
type AccessRequest struct {
	Slug         string `json:"slug"`
	TournamentID string `json:"tournamentID"`
	Email        string `json:"email"`
}
