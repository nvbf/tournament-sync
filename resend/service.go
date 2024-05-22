package resend

import (
	"context"
	"fmt"
	"log"
	"os"

	"cloud.google.com/go/firestore"
	resend "github.com/resend/resend-go/v2"
)

// Service represents the migration status of a single service.
type Service struct {
	firebaseClient *firestore.Client
	rebaseClient   *resend.Client
}

// NewService creates a new empty service.
func NewService(firestoreClient *firestore.Client) *Service {
	resendKey := os.Getenv("RESEND_KEY")
	return &Service{
		firebaseClient: firestoreClient,
		rebaseClient:   resend.NewClient(resendKey),
	}
}

func (s Service) SendMail(ctx context.Context, request AccessRequest, accessCode string) error {
	body := fmt.Sprintf("<a>https://scoreboard-sandbox.herokuapp.com/get-access/%s</a>", accessCode)
	params := &resend.SendEmailRequest{
		From:    "onboarding@resend.dev",
		To:      []string{"oysteigr@gmail.com"},
		Subject: "Hello Admin",
		Html:    body,
	}

	_, err := s.rebaseClient.Emails.Send(params)
	if err != nil {
		log.Fatalf("Failed to send mail request: %v", err)
		return err
	}
	return nil
}

func (s Service) GrantAccess(ctx context.Context, slug, userID string) error {
	// Get a reference to the document
	docRef := s.firebaseClient.Collection("TournementAdmin").Doc(slug)

	// Transaction to ensure that the update is atomic
	// attempt to retrieve the document
	// Check if UID already has access
	// User already has access
	// Add UID to the authorized users array if not present
	err := grantAccessToDoc(ctx, s, docRef, userID)

	if err != nil {
		log.Printf("Failed to update document: %v", err)
		return err
	}

	return nil
}

func grantAccessToDoc(ctx context.Context, s Service, docRef *firestore.DocumentRef, userID string) error {
	err := s.firebaseClient.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		doc, err := tx.Get(docRef)
		if err != nil {
			return err
		}

		var allowedUsers []string
		if data, err := doc.DataAt("allowedUsers"); err == nil {
			allowedUsers = data.([]string)
		}

		for _, user := range allowedUsers {
			if user == userID {
				return nil
			}
		}

		updatedUsers := append(allowedUsers, userID)
		return tx.Update(docRef, []firestore.Update{
			{Path: "allowedUsers", Value: updatedUsers},
		})
	})
	return err
}
