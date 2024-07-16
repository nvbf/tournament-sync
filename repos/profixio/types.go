package profixio

type MatchResponse struct {
	Data []Match `json:"data"`
	Meta struct {
		CurrentPage int `json:"current_page"`
		From        int `json:"from"`
		LastPage    int `json:"last_page"`
	} `json:"meta"`
}

type TournamentResponse struct {
	Data  []Tournament `json:"data"`
	Links struct {
		First string `json:"first"`
		Last  string `json:"last"`
		Prev  string `json:"prev"`
		Next  string `json:"next"`
	} `json:"links"`
	Meta struct {
		CurrentPage int `json:"current_page"`
		From        int `json:"from"`
		LastPage    int `json:"last_page"`
	} `json:"meta"`
}

type Tournament struct {
	ID           *int    `json:"id"`
	Name         *string `json:"name"`
	Type         *string `json:"type"`
	Slug         *string `json:"slug"`
	StartDate    *string `json:"startDate"`
	EndDate      *string `json:"endDate"`
	StatsWritten bool    `json:"StatsWritten"`
}

type TournamentPublic struct {
	Name         *string `json:"name"`
	Type         *string `json:"type"`
	Slug         *string `json:"slug"`
	StartDate    *string `json:"startDate"`
	EndDate      *string `json:"endDate"`
	StatsWritten bool    `json:"StatsWritten"`
}

type TournamentSecrets struct {
	ID     *int    `json:"id"`
	Slug   *string `json:"slug"`
	Secret *string `json:"secret"`
}

type CustomTournament struct {
	Slug    *string  `json:"slug"`
	Matches *[]Match `json:"matches"`
}

type Match struct {
	ID                         *int64     `json:"id"`
	Txid                       *int       `json:"txid"`
	Number                     *string    `json:"number"`
	TournamentID               *int       `json:"tournamentId"`
	Name                       *string    `json:"name"`
	GameRound                  *int       `json:"gameRound"`
	Date                       *string    `json:"date"`
	Time                       *string    `json:"time"`
	HomeTeam                   *Team      `json:"homeTeam"`
	AwayTeam                   *Team      `json:"awayTeam"`
	HasWinner                  *bool      `json:"hasWinner"`
	WinnerTeam                 *string    `json:"winnerTeam"`
	Field                      *Field     `json:"field"`
	IsHidden                   *bool      `json:"isHidden"`
	IsGroupPlay                *bool      `json:"isGroupPlay"`
	IsPlayoff                  *bool      `json:"isPlayoff"`
	PlayoffLevel               *int       `json:"playoffLevel"`
	IncludedInTableCalculation *bool      `json:"includedInTableCalculation"`
	MatchGroup                 *Group     `json:"matchGroup"`
	MatchCategory              *Category  `json:"matchCategory"`
	SettResultsFormatted       *string    `json:"settResultsFormatted"`
	Sets                       *[]Set     `json:"sets"`
	RefereesTX                 *[]Referee `json:"refereesTX"`
	MatchDataUpdated           *string    `json:"matchDataUpdated"`
	ResultsUpdated             *string    `json:"resultsUpdated"`
}

type Team struct {
	TeamRegistrationID int         `json:"teamRegistrationId"`
	GlobalTeamID       interface{} `json:"globalTeamId"`
	Name               string      `json:"name"`
	Goals              int         `json:"goals"`
	IsWinner           bool        `json:"isWinner"`
	Seeding            int         `json:"seeding"`
}

type Field struct {
	ID    *int    `json:"id"`
	Name  *string `json:"name"`
	Arena *Arena  `json:"arena"`
}

type Arena struct {
	ID        *int    `json:"id"`
	ArenaName *string `json:"arenaName"`
}

type Group struct {
	ID          *int    `json:"id"`
	DisplayName *string `json:"displayName"`
	Name        *string `json:"name"`
}

type Category struct {
	ID           *int    `json:"id"`
	Name         *string `json:"name"`
	CategoryCode *string `json:"categoryCode"`
}

type Set struct {
	Number         *int `json:"number"`
	PointsHomeTeam *int `json:"pointsHomeTeam"`
	PointsAwayTeam *int `json:"pointsAwayTeam"`
}

type Referee struct {
	RefereeLevel *int    `json:"refereeLevel"`
	Text         *string `json:"text"`
	TxName       *string `json:"txName"`
}

type MatchResult struct {
	Sets   []Result `json:"sets"`
	Result Result   `json:"result"`
}

type Result struct {
	Home int `json:"home"`
	Away int `json:"away"`
}
