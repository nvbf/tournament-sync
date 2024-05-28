package auth

import (
	"context"
	"net/http"

	firebase "firebase.google.com/go/v4"

	"github.com/gin-gonic/gin"
)

func AuthMiddleware(firebaseApp *firebase.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header is missing"})
			c.Abort()
			return
		}
		idToken := authHeader[len("Bearer "):]

		ctx := context.Background()
		authClient, err := firebaseApp.Auth(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to initialize Firebase Auth"})
			c.Abort()
			return
		}

		token, err := authClient.VerifyIDToken(ctx, idToken)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid ID token"})
			c.Abort()
			return
		}

		// Attach token to the context
		c.Set("token", token)

		c.Next()
	}
}
