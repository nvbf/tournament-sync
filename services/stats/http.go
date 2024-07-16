package stats

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Router is the interface for a router.
type Router interface {
	GET(relativePath string, handlers ...gin.HandlerFunc) gin.IRoutes
	Use(middleware ...gin.HandlerFunc) gin.IRoutes
	Group(relativePath string, handlers ...gin.HandlerFunc) *gin.RouterGroup
}

// Greeter is the interface for a greeter service.
type Stats interface {
	GetStats(c *gin.Context) ([]*TournamentStats, error)
	UpdateStats(c *gin.Context) error
}

// HTTPOptions contains all the options needed for the HTTP handler.
type HTTPOptions struct {

	// The service we provides the HTTP transport for.
	Service Stats

	// The router instance to configure the HTTP routes.
	Router Router
}

// NewHTTPHandler creates a new HTTP handler.
func NewHTTPHandler(opts HTTPOptions) {
	r := opts.Router
	h := &httpHandler{opts}
	r.GET("/all", h.getStatsHandler)
	r.GET("/update", h.updateStatsHandler)
}

type httpHandler struct {
	HTTPOptions
}

func (s *httpHandler) getStatsHandler(c *gin.Context) {

	stats, err := s.Service.GetStats(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "something went wrong"})
		c.Abort()
		return
	}

	c.JSON(http.StatusOK, gin.H{"stats": stats})
	s.Service.GetStats(c)
}

func (s *httpHandler) updateStatsHandler(c *gin.Context) {
	err := s.Service.UpdateStats(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "something went wrong"})
		c.Abort()
		return
	}
}
