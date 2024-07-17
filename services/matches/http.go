package matches

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	profixio "github.com/nvbf/tournament-sync/repos/profixio"
)

// Router is the interface for a router.
type Router interface {
	GET(relativePath string, handlers ...gin.HandlerFunc) gin.IRoutes
	POST(relativePath string, handlers ...gin.HandlerFunc) gin.IRoutes
	Use(middleware ...gin.HandlerFunc) gin.IRoutes
	Group(relativePath string, handlers ...gin.HandlerFunc) *gin.RouterGroup
}

// Greeter is the interface for a greeter service.
type Results interface {
	ReportResult(c *gin.Context, matchID string) error
}

// HTTPOptions contains all the options needed for the HTTP handler.
type HTTPOptions struct {

	// The service we provides the HTTP transport for.
	Service Results

	// The router instance to configure the HTTP routes.
	Router Router
}

// NewHTTPHandler creates a new HTTP handler.
func NewHTTPHandler(opts HTTPOptions) {
	r := opts.Router
	h := &httpHandler{opts}
	r.GET("/result/:match_id", h.resultHandler)
}

type httpHandler struct {
	HTTPOptions
}

func (h *httpHandler) resultHandler(c *gin.Context) {
	matchID := c.Param("match_id")

	err := h.Service.ReportResult(c, matchID)
	if err != nil {
		if err == profixio.ErrAlreadyRegistered {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			c.Abort()
			return
		}
		log.Printf("Could not register result: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "something went wrong"})
		c.Abort()
		return
	}
	c.JSON(http.StatusAccepted, gin.H{
		"message": "Result registered",
	})
}
