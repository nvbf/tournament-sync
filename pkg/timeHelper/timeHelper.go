package timehelper

import "time"

func GetTodaysDateString() string {
	// Get the current date
	currentTime := time.Now()

	// Format the date to 'YYYY-MM-DD'
	return currentTime.Format("2006-01-02")
}
