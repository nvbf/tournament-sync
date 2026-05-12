package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"
	access "github.com/nvbf/tournament-sync/pkg/accessCode"
	log "github.com/nvbf/tournament-sync/pkg/cloudlog"
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
	log.Info("request start", log.WithRequest(c, log.Fields{"handler": "claim", "path": c.FullPath()}))

	var request resend.AccessRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		log.Warning("request invalid", log.WithRequest(c, log.Fields{"handler": "claim", "path": c.FullPath(), "reason": "invalid_body"}))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		c.Abort()
		return
	}

	err := s.Service.ClaimAccess(c, request)
	if err != nil {
		if err == ErrInvalidTournementID {
			log.Warning("request invalid", log.WithRequest(c, log.Fields{"handler": "claim", "path": c.FullPath(), "slug": request.Slug, "reason": "invalid_tournament_id"}))
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			c.Abort()
			return
		}
		log.Error("request failed", err, log.WithRequest(c, log.Fields{"handler": "claim", "path": c.FullPath(), "slug": request.Slug}))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		c.Abort()
		return
	}
	log.Info("request completed", log.WithRequest(c, log.Fields{"handler": "claim", "path": c.FullPath(), "slug": request.Slug}))

	c.JSON(http.StatusOK, gin.H{
		"result":       "Access granted",
		"slug":         request.Slug,
		"tournamentID": request.TournamentID,
		"email":        request.Email,
	})

}

func (s *httpHandler) accessHandler(c *gin.Context) {
	accessCode := c.Param("access_code")
	log.Info("request start", log.WithRequest(c, log.Fields{"handler": "access", "path": c.FullPath()}))
	slug, uniqueID, err := access.Decode(accessCode)
	if err != nil {
		log.Error("request failed", err, log.WithRequest(c, log.Fields{"handler": "access", "path": c.FullPath(), "reason": "decode_access_code"}))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid access code"})
		c.Abort()
		return
	}

	err = s.Service.AddTournamentAccess(c, slug, uniqueID)
	if err != nil {
		log.Warning("request forbidden", log.WithRequest(c, log.Fields{"handler": "access", "path": c.FullPath(), "slug": slug}))
		c.JSON(http.StatusForbidden, gin.H{"error": "not valid access code"})
		c.Abort()
		return
	}
	log.Info("request completed", log.WithRequest(c, log.Fields{"handler": "access", "path": c.FullPath(), "slug": slug}))
	c.JSON(http.StatusOK, gin.H{"slug": slug})
}
