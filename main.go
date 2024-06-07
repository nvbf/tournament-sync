package main

import (
	"context"
	"log"
	"os"
	"strings"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go/v4"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"google.golang.org/api/option"

	profixio "github.com/nvbf/tournament-sync/repos/profixio"
	resend "github.com/nvbf/tournament-sync/repos/resend"

	auth "github.com/nvbf/tournament-sync/pkg/auth"

	admin "github.com/nvbf/tournament-sync/services/admin"
	matches "github.com/nvbf/tournament-sync/services/matches"
	sync "github.com/nvbf/tournament-sync/services/sync"
)

func main() {
	ctx := context.Background()

	projectID := os.Getenv("FIREBASE_PROJECT_ID")
	credentialsJSON := os.Getenv("FIREBASE_CREDENTIALS_JSON")
	port := os.Getenv("PORT")
	allowOrigins := os.Getenv("CORS_HOSTS")

	credentialsOption := option.WithCredentialsJSON([]byte(credentialsJSON))

	firestoreClient, err := firestore.NewClient(ctx, projectID, credentialsOption)
	if err != nil {
		log.Fatalf("Failed to create Firestore client: %v", err)
	}
	defer firestoreClient.Close()

	firebaseApp, err := firebase.NewApp(ctx, nil, credentialsOption)
	if err != nil {
		log.Fatalf("error initializing app: %v\n", err)
	}

	profixioService := profixio.NewService(firestoreClient)
	resendService := resend.NewService(firestoreClient)

	adminService := admin.NewAdminService(firestoreClient, firebaseApp, resendService)
	syncService := sync.NewSyncService(firestoreClient, firebaseApp, profixioService)
	matchesService := matches.NewMatchesService(firestoreClient, firebaseApp, profixioService)

	config := cors.DefaultConfig()
	config.AllowOrigins = strings.Split(allowOrigins, ",")
	config.AllowCredentials = true
	config.AllowMethods = []string{"GET", "POST", "OPTIONS"}
	config.AllowHeaders = []string{"Origin", "Content-Length", "Content-Type", "Authorization", "Access-Control-Allow-Origin"}

	router := gin.Default()
	// router.Use(cors.New(config))

	adminRouter := router.Group("/admin/v1")
	adminRouter.Use(auth.AuthMiddleware(firebaseApp)) // Apply the middleware here

	matchesRouter := router.Group("/matches/v1")
	matchesRouter.Use(auth.AuthMiddleware(firebaseApp)) // Apply the middleware here

	syncRouter := router.Group("/sync/v1")

	admin.NewHTTPHandler(admin.HTTPOptions{
		Service: adminService,
		Router:  adminRouter,
	})

	matches.NewHTTPHandler(matches.HTTPOptions{
		Service: matchesService,
		Router:  matchesRouter,
	})

	sync.NewHTTPHandler(sync.HTTPOptions{
		Service: syncService,
		Router:  syncRouter,
	})

	log.Fatal(router.Run(":" + port))
}
