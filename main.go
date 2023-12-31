package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"google.golang.org/api/option"

	profixio "github.com/nvbf/tournament-sync/profixio"
)

func main() {

	// Initialize Firestore client
	ctx := context.Background()

	// Get Firebase configuration from environment variables
	projectID := os.Getenv("FIREBASE_PROJECT_ID")
	credentialsJSON := os.Getenv("FIREBASE_CREDENTIALS_JSON")
	port := os.Getenv("PORT")
	allowOrigins := os.Getenv("CORS_HOSTS")

	// Create an option with the credentials JSON as a byte array
	credentialsOption := option.WithCredentialsJSON([]byte(credentialsJSON))

	firestoreClient, err := firestore.NewClient(ctx, projectID, credentialsOption)
	if err != nil {
		log.Fatalf("Failed to create Firestore client: %v", err)
	}
	defer firestoreClient.Close()

	service := profixio.NewService(firestoreClient)

	// setup CORS
	config := cors.DefaultConfig()
	config.AllowOrigins = strings.Split(allowOrigins, ",") // replace with your client's URL
	config.AllowCredentials = true
	config.AllowMethods = []string{"GET"}
	config.AllowHeaders = []string{"Origin", "Content-Length", "Content-Type"}

	// Create router
	router := gin.Default()
	router.Use(cors.New(config))

	router.GET("/sync/v1/tournaments", func(c *gin.Context) {

		// Start the asynchronous function
		go service.FetchTournaments(ctx, 1)

		c.JSON(http.StatusOK, gin.H{
			"message": "Async function started",
		})
	})

	router.GET("/sync/v1/tournament/:slug_id", func(c *gin.Context) {
		slugID := c.Param("slug_id")
		layout := "2006-01-02 15:04:05"

		t := time.Now()
		now := t.Format(layout)

		lastSync := service.GetLastSynced(ctx, slugID)
		lastReq := service.GetLastRequest(ctx, slugID)
		if lastReq == "" {
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
			service.SetLastRequest(ctx, slugID, now)
			// Start the asynchronous function
			go service.FetchMatches(ctx, 1, slugID, lastSync, now)

			c.JSON(http.StatusOK, gin.H{
				"message": fmt.Sprintf("Async function started sync from lastSync: %s", lastSync),
			})
		}
	})

	log.Fatal(router.Run(":" + port))
}
