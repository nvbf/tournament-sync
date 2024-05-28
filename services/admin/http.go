package admin

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	access "github.com/nvbf/tournament-sync/pkg/accessCode"
	resend "github.com/nvbf/tournament-sync/repos/resend"
)

// Router is the interface for a router.
type Router interface {
	GET(relativePath string, handlers ...gin.HandlerFunc) gin.IRoutes
	POST(relativePath string, handlers ...gin.HandlerFunc) gin.IRoutes
	Use(middleware ...gin.HandlerFunc) gin.IRoutes
	Group(relativePath string, handlers ...gin.HandlerFunc) *gin.RouterGroup
}

// Greeter is the interface for a greeter service.
type Admin interface {
	ClaimAccess(c *gin.Context, request resend.AccessRequest) error
	AddTournamentAccess(c *gin.Context, slug, uniruqID string) error
}

// HTTPOptions contains all the options needed for the HTTP handler.
type HTTPOptions struct {

	// The service we provides the HTTP transport for.
	Service Admin

	// The router instance to configure the HTTP routes.
	Router Router
}

// NewHTTPHandler creates a new HTTP handler.
func NewHTTPHandler(opts HTTPOptions) {
	r := opts.Router
	h := &httpHandler{opts}
	r.POST("/claim", h.claimHandler)
	r.GET("/access/:access_code", h.accessHandler)
}

type httpHandler struct {
	HTTPOptions
}

func (s *httpHandler) claimHandler(c *gin.Context) {

	var request resend.AccessRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		c.Abort()
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"result":       "Access granted",
		"slug":         request.Slug,
		"tournamentID": request.TournamentID,
		"email":        request.Email,
	})

}

func (s *httpHandler) accessHandler(c *gin.Context) {
	accessCode := c.Param("access_code")
	slug, uniqueID, err := access.Decode(accessCode)
	if err != nil {
		log.Printf("Failed to decode access code: %v\n", err)
		return
	}

	err = s.Service.AddTournamentAccess(c, slug, uniqueID)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "not valid access code"})
		c.Abort()
		return
	}
	c.JSON(http.StatusOK, gin.H{"slug": slug})
}
