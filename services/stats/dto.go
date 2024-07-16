package stats

type Event struct {
	Author    string `firestore:"author"`
	EventType string `firestore:"eventType"`
	ID        string `firestore:"id"`
	PlayerID  int    `firestore:"playerId"`
	Reference string `firestore:"reference"`
	Team      string `firestore:"team"`
	Timestamp int64  `firestore:"timestamp"`
	Undone    string `firestore:"undone"`
}

type Tournament struct {
	Name      string  `firestore:"Name"`
	Type      string  `firestore:"Type"`
	Slug      string  `firestore:"Slug"`
	StartDate string  `firestore:"StartDate"`
	EndDate   string  `firestore:"EndDate"`
	Matches   []Match `firestore:"Matches"`
}

type TournamentStats struct {
	Name                string  `firestore:"Name"`
	Slug                string  `firestore:"Slug"`
	StartDate           string  `firestore:"StartDate"`
	EndDate             string  `firestore:"EndDate"`
	Matches             []Match `firestore:"Matches"`
	NumberOfScoreboards int     `firestore:"NumberOfScoreboards"`
	NumberOfMatches     int     `firestore:"NumberOfMatches"`
	StatsWritten        bool    `firestore:"StatsWritten"`
}

type Match struct {
	ScoreboardId string `firestore:"ScoreboardId"`
	Number       string `firestore:"Number"`
}
