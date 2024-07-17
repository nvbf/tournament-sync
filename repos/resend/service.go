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
	hostURL        string
}

// NewService creates a new empty service.
func NewService(firestoreClient *firestore.Client, hostURL string) *Service {
	resendKey := os.Getenv("RESEND_KEY")
	return &Service{
		firebaseClient: firestoreClient,
		rebaseClient:   resend.NewClient(resendKey),
		hostURL:        hostURL,
	}
}

func (s Service) SendMail(ctx context.Context, request AccessRequest, accessCode string) error {
	body := getEmailTemplate(fmt.Sprintf("%s/get-access/%s", s.hostURL, accessCode))
	// body := fmt.Sprintf("<a>%s/get-access/%s</a>", s.hostURL, accessCode)
	params := &resend.SendEmailRequest{
		From:    "onboarding@resend.dev",
		To:      []string{request.Email},
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
	docRef := s.firebaseClient.Collection("TournamentSecrets").Doc(slug)

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
		// Retrieve the allowedUsers field from the document, if it exists
		if data, err := doc.DataAt("allowedUsers"); err == nil {
			// Type assert the data to a slice of interface{}
			if users, ok := data.([]interface{}); ok {
				// Convert the slice of interface{} to a slice of strings
				for _, user := range users {
					if userStr, ok := user.(string); ok {
						allowedUsers = append(allowedUsers, userStr)
					}
				}
			}
		}

		// Check if the userID already exists in the allowedUsers list
		for _, user := range allowedUsers {
			if user == userID {
				// User already has access, so return nil to indicate no update needed
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

func getEmailTemplate(url string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <style>
        body {
            font-family: Arial, sans-serif;
            background-color: #f4f4f4;
            margin: 0;
            padding: 20px;
        }
        .container {
            background-color: #ffffff;
            width: 1;
            max-width: 600px;
            margin: 0 auto;
            padding: 20px;
            box-shadow: 0 0 10px rgba(0,0,0,0.1);
        }
        .button {
            display: block;
            width: 200px;
            height: 50px;
            margin: 20px auto;
            background-color: #007BFF;
            color: #ffffff;
            font-size: 16px;
            text-align: center;
            line-height: 50px;
            text-decoration: none;
            border-radius: 5px;
        }
        .button:hover {
            background-color: #0056b3;
        }
    </style>
</head>
<body>
    <div class="container">
        <h2>Hello,</h2>
        <p>Thank you for using our service. Please click the button below to proceed:</p>
        <a href="%s" class="button">Click Here</a>
        <p>Best regards,<br>Your Company Name</p>
    </div>
</body>
</html>`, url)
}
