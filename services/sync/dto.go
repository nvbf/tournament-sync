package sync

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