package admin

import (
	"errors"
	"log"
	"net/http"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go/v4"
	auth "firebase.google.com/go/v4/auth"

	"github.com/gin-gonic/gin"
	access "github.com/nvbf/tournament-sync/pkg/accessCode"
	resend "github.com/nvbf/tournament-sync/repos/resend"
)

type AdminService struct {
	firestoreClient *firestore.Client
	firebaseApp     *firebase.App
	resendService   *resend.Service
}

func NewAdminService(firestoreClient *firestore.Client, firebaseApp *firebase.App, resendService *resend.Service) *AdminService {
	return &AdminService{
		firestoreClient: firestoreClient,
		firebaseApp:     firebaseApp,
		resendService:   resendService,
	}
}

func (s *AdminService) ClaimAccess(c *gin.Context, request resend.AccessRequest) error {
	token := c.MustGet("token").(*auth.Token)

	doc, err := s.firestoreClient.Collection("TournamentSecrets").Doc(request.Slug).Get(c)
	if err != nil {
		log.Printf("Failed to get tournament to Firestore: %v\n", err)
		return err
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

	err = s.resendService.SendMail(c, request, accessCode)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to send mail request"})
		c.Abort()
		return err
	}

	go s.resendService.GrantAccess(c, request.Slug, token.UID)
	return nil
}

func (s *AdminService) AddTournamentAccess(c *gin.Context, slug, uniqueID string) error {
	token := c.MustGet("token").(*auth.Token)

	doc, err := s.firestoreClient.Collection("TournamentSecrets").Doc(slug).Get(c)
	if err != nil {
		log.Printf("Failed to get tournament to Firestore: %v\n", err)
		return err
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

	if uniqueID == secretString {
		s.resendService.GrantAccess(c, slug, token.UID)
	} else {
		c.JSON(http.StatusForbidden, gin.H{"error": "not valid access code"})
		c.Abort()
		return errors.New("not valid access code")
	}
	return nil
}
