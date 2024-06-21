package sync

import (
	"fmt"
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
	SyncTournamentMatches(c *gin.Context, slug string, force bool) error
	UpdateCustomTournament(c *gin.Context, slug string, tournament profixio.CustomTournament) error
	CreateIfNoExisting(c *gin.Context, slug string) error
	GetStats(c *gin.Context) error
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
	r.GET("/stats", h.getStatsHandler)
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

	// Parse the URL query parameters
	c.Request.ParseForm()
	// Get the 'force' query parameter
	forceParam := c.Request.Form.Get("force")
	if forceParam != "" {
		fmt.Printf("The 'force' parameter value is: %s\n", forceParam)
	} else {
		fmt.Printf("The 'force' parameter is not present in the URL.\n")
	}
	err := s.Service.SyncTournamentMatches(c, slug, forceParam == "true")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "something went wrong"})
		c.Abort()
		return
	}
	c.JSON(http.StatusOK, gin.H{"slug": slug})
}

func (s *httpHandler) updateCustomTournamentHandler(c *gin.Context) {
	// slug := c.Param("slug_id")
	slug := "nevza_oddanesand_24"

	err := s.Service.CreateIfNoExisting(c, slug)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "something went wrong"})
		c.Abort()
		return
	}

	var request profixio.CustomTournament
	if err = c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		c.Abort()
		return
	}

	err = s.Service.UpdateCustomTournament(c, slug, request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "something went wrong"})
		c.Abort()
		return
	}
	c.JSON(http.StatusOK, gin.H{"slug": slug})
}

func (s *httpHandler) getStatsHandler(c *gin.Context) {
	s.Service.GetStats(c)
}
