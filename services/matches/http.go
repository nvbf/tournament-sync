package matches

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	profixio "github.com/nvbf/tournament-sync/repos/profixio"
)

// Router is the interface for a router.
type Router interface {
	GET(relativePath string, handlers ...gin.HandlerFunc) gin.IRoutes
	POST(relativePath string, handlers ...gin.HandlerFunc) gin.IRoutes
	PUT(relativePath string, handlers ...gin.HandlerFunc) gin.IRoutes
	Use(middleware ...gin.HandlerFunc) gin.IRoutes
	Group(relativePath string, handlers ...gin.HandlerFunc) *gin.RouterGroup
}

// Greeter is the interface for a greeter service.
type Results interface {
	ReportResult(c *gin.Context, matchID string) error
	FinalizeResult(c *gin.Context, matchID string) error
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
	r.PUT("/result/finalize/:match_id", h.finalizeResultHandler)
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

func (h *httpHandler) finalizeResultHandler(c *gin.Context) {
	matchID := c.Param("match_id")

	err := h.Service.FinalizeResult(c, matchID)
	if err != nil {
		if errors.Is(err, ErrFinalizeTooSoon) {
			setFinalizeRetryHeaders(c, err)
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			c.Abort()
			return
		}
		switch {
		case errors.Is(err, ErrInvalidMatchResult), errors.Is(err, ErrNoEventsToFinalize):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			c.Abort()
			return
		case errors.Is(err, ErrMatchAlreadyFinalized), errors.Is(err, profixio.ErrAlreadyRegistered):
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			c.Abort()
			return
		default:
			log.Printf("Could not finalize result: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "something went wrong"})
			c.Abort()
			return
		}
	}

	err = h.Service.ReportResult(c, matchID)
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
		"message": "Result finalized and registered",
	})
}

func setFinalizeRetryHeaders(c *gin.Context, err error) {
	tooSoonErr := &FinalizeTooSoonError{}
	if !errors.As(err, &tooSoonErr) {
		return
	}

	remaining := time.Until(tooSoonErr.RetryAt)
	if remaining < 0 {
		remaining = 0
	}
	remainingSeconds := int((remaining + time.Second - 1) / time.Second)

	c.Header("Retry-After", strconv.Itoa(remainingSeconds))
	c.Header("X-Retry-At", tooSoonErr.RetryAt.UTC().Format(time.RFC3339))
}
