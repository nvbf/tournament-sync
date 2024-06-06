package sync

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nvbf/tournament-sync/repos/profixio"
)

// Router is the interface for a router.
type Router interface {
	GET(relativePath string, handlers ...gin.HandlerFunc) gin.IRoutes
	POST(relativePath string, handlers ...gin.HandlerFunc) gin.IRoutes
	Use(middleware ...gin.HandlerFunc) gin.IRoutes
	Group(relativePath string, handlers ...gin.HandlerFunc) *gin.RouterGroup
}

// Greeter is the interface for a greeter service.
type Sync interface {
	FetchTournaments(c *gin.Context) error
	SyncTournamentMatches(c *gin.Context, slug string) error
	UpdateCustomTournament(c *gin.Context, slug string, tournament profixio.CustomTournament) error
}

// HTTPOptions contains all the options needed for the HTTP handler.
type HTTPOptions struct {

	// The service we provides the HTTP transport for.
	Service Sync

	// The router instance to configure the HTTP routes.
	Router Router
}

// NewHTTPHandler creates a new HTTP handler.
func NewHTTPHandler(opts HTTPOptions) {
	r := opts.Router
	h := &httpHandler{opts}
	r.GET("/tournaments", h.syncTournamentsHandler)
	r.GET("/tournament/:slug_id", h.syncTournamentMatchesHandler)
	r.POST("/custom/tournament/:slug_id", h.updateCustomTournamentHandler)
}

type httpHandler struct {
	HTTPOptions
}

func (s *httpHandler) syncTournamentsHandler(c *gin.Context) {
	err := s.Service.FetchTournaments(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "something went wrong"})
		c.Abort()
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "Async function started",
	})
}

func (s *httpHandler) syncTournamentMatchesHandler(c *gin.Context) {
	slug := c.Param("slug_id")

	err := s.Service.SyncTournamentMatches(c, slug)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "something went wrong"})
		c.Abort()
		return
	}
	c.JSON(http.StatusOK, gin.H{"slug": slug})
}

func (s *httpHandler) updateCustomTournamentHandler(c *gin.Context) {
	// slug := c.Param("slug_id")
	slug := "aa_test_csv"

	var request profixio.CustomTournament
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		c.Abort()
		return
	}

	err := s.Service.UpdateCustomTournament(c, slug, request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "something went wrong"})
		c.Abort()
		return
	}
	c.JSON(http.StatusOK, gin.H{"slug": slug})
}
