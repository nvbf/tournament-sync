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
	firebase "firebase.google.com/go/v4"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"google.golang.org/api/option"

	access "github.com/nvbf/tournament-sync/pkg/accessCode"
	profixio "github.com/nvbf/tournament-sync/profixio"
	resend "github.com/nvbf/tournament-sync/resend"
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
	firesbaseApp, err := firebase.NewApp(ctx, nil, credentialsOption)
	if err != nil {
		log.Fatalf("error initializing app: %v\n", err)
	}

	profixioService := profixio.NewService(firestoreClient)
	resendService := resend.NewService(firestoreClient)

	// setup CORS
	config := cors.DefaultConfig()
	config.AllowOrigins = strings.Split(allowOrigins, ",") // replace with your client's URL
	config.AllowCredentials = true
	config.AllowMethods = []string{"GET", "POST"}
	config.AllowHeaders = []string{"Origin", "Content-Length", "Content-Type"}

	// Create router
	router := gin.Default()
	router.Use(cors.New(config))

	router.GET("/sync/v1/tournaments", func(c *gin.Context) {

		// Start the asynchronous function
		go profixioService.FetchTournaments(ctx, 1)

		c.JSON(http.StatusOK, gin.H{
			"message": "Async function started",
		})
	})

	router.GET("/sync/v1/tournament/:slug_id", func(c *gin.Context) {
		slugID := c.Param("slug_id")
		layout := "2006-01-02 15:04:05"

		t := time.Now()
		t_m := time.Now().Add(-1 * time.Minute)
		now := t.Format(layout)
		now_m := t_m.Format(layout)

		lastSync := profixioService.GetLastSynced(ctx, slugID)
		lastReq := profixioService.GetLastRequest(ctx, slugID)
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
			profixioService.SetLastRequest(ctx, slugID, now)
			// Start the asynchronous function
			go profixioService.FetchMatches(ctx, 1, slugID, lastSync, now_m)

			c.JSON(http.StatusOK, gin.H{
				"message": fmt.Sprintf("Async function started sync from lastSync: %s", lastSync),
			})
		}
	})

	router.POST("admin/v1/claim", func(c *gin.Context) {
		// Assume the token is sent as a Bearer token in the Authorization header
		authHeader := c.GetHeader("Authorization")
		idToken := authHeader[len("Bearer "):]

		// Initialize Firebase Auth
		ctx := context.Background()
		authClient, err := firesbaseApp.Auth(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to initialize Firebase Auth"})
			c.Abort()
			return
		}

		// Verify ID Token
		token, err := authClient.VerifyIDToken(ctx, idToken)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid ID token"})
			c.Abort()
			return
		}

		var request resend.AccessRequest

		// Bind the JSON to the AccessRequest struct
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Write the tournament to Firestore
		doc, err := firestoreClient.Collection("TournamentSecrets").Doc(request.Slug).Get(ctx)
		if err != nil {
			log.Printf("Failed to get tournament to Firestore: %v\n", err)
			return
		}

		data := doc.Data()
		fieldValue, ok := data["Secret"]
		if !ok {
			log.Printf("Field does not exist in the document.")
		}

		secretString, ok := fieldValue.(string)
		if !ok {
			log.Printf("Failed to convert field value to string.")
		}

		accessCode := access.GenerateCode(request.Slug, secretString)

		// Assuming resendService.SendMail is properly defined and ctx is available
		err = resendService.SendMail(ctx, request, accessCode)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to send mail request"})
			c.Abort()
			return
		}

		// Respond back with success message
		c.JSON(http.StatusOK, gin.H{
			"result":       "Access granted",
			"slug":         request.Slug,
			"tournamentID": request.TournamentID,
			"email":        request.Email,
		})

		go resendService.GrantAccess(ctx, request.Slug, token.UID)
	})

	router.GET("admin/v1/access/:access_code", func(c *gin.Context) {
		// Assume the token is sent as a Bearer token in the Authorization header
		authHeader := c.GetHeader("Authorization")
		idToken := authHeader[len("Bearer "):]

		// Initialize Firebase Auth
		ctx := context.Background()
		authClient, err := firesbaseApp.Auth(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to initialize Firebase Auth"})
			c.Abort()
			return
		}

		// Verify ID Token
		token, err := authClient.VerifyIDToken(ctx, idToken)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid ID token"})
			c.Abort()
			return
		}

		accessCode := c.Param("access_code")

		slug, uniqueId, err := access.Decode(accessCode)
		if err != nil {
			log.Printf("Failed to decode access code: %v\n", err)
			return
		}

		// Write the tournament to Firestore
		doc, err := firestoreClient.Collection("TournamentSecrets").Doc(slug).Get(ctx)
		if err != nil {
			log.Printf("Failed to get tournament to Firestore: %v\n", err)
			return
		}

		data := doc.Data()
		fieldValue, ok := data["Secret"]
		if !ok {
			log.Printf("Field does not exist in the document.")
		}

		secretString, ok := fieldValue.(string)
		if !ok {
			log.Printf("Failed to convert field value to string.")
		}

		if err != nil {
			fmt.Println(err)
		}

		if uniqueId == secretString {
			resendService.GrantAccess(ctx, slug, token.UID)
			c.Redirect(http.StatusFound, "/tournamentadmin/"+slug)
		} else {
			c.JSON(http.StatusForbidden, gin.H{"error": "not valid access code"})
			c.Abort()
			return
		}
	})

	log.Fatal(router.Run(":" + port))
}
