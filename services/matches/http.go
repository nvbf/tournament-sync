package matches

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	log "github.com/nvbf/tournament-sync/pkg/cloudlog"

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
	log.Info("request start", log.WithRequest(c, log.Fields{"handler": "result", "path": c.FullPath(), "matchID": matchID}))

	err := h.Service.ReportResult(c, matchID)
	if err != nil {
		if err == profixio.ErrAlreadyRegistered {
			log.Warning("request conflict", log.WithRequest(c, log.Fields{"handler": "result", "path": c.FullPath(), "matchID": matchID, "reason": "already_registered"}))
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			c.Abort()
			return
		}
		log.Error("request failed", err, log.WithRequest(c, log.Fields{"handler": "result", "path": c.FullPath(), "matchID": matchID}))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "something went wrong"})
		c.Abort()
		return
	}
	log.Info("request completed", log.WithRequest(c, log.Fields{"handler": "result", "path": c.FullPath(), "matchID": matchID}))
	c.JSON(http.StatusAccepted, gin.H{
		"message": "Result registered",
	})
}

func (h *httpHandler) finalizeResultHandler(c *gin.Context) {
	matchID := c.Param("match_id")
	log.Info("request start", log.WithRequest(c, log.Fields{"handler": "finalizeResult", "path": c.FullPath(), "matchID": matchID}))

	err := h.Service.FinalizeResult(c, matchID)
	if err != nil {
		if errors.Is(err, ErrFinalizeTooSoon) {
			log.Warning("request invalid", log.WithRequest(c, log.Fields{"handler": "finalizeResult", "path": c.FullPath(), "matchID": matchID, "reason": "finalize_too_soon"}))
			setFinalizeRetryHeaders(c, err)
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			c.Abort()
			return
		}
		switch {
		case errors.Is(err, ErrInvalidMatchResult), errors.Is(err, ErrNoEventsToFinalize):
			log.Warning("request invalid", log.WithRequest(c, log.Fields{"handler": "finalizeResult", "path": c.FullPath(), "matchID": matchID, "reason": "invalid_match_result"}))
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			c.Abort()
			return
		case errors.Is(err, ErrMatchAlreadyFinalized), errors.Is(err, profixio.ErrAlreadyRegistered):
			log.Warning("request conflict", log.WithRequest(c, log.Fields{"handler": "finalizeResult", "path": c.FullPath(), "matchID": matchID, "reason": "already_finalized_or_registered"}))
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			c.Abort()
			return
		default:
			log.Error("request failed", err, log.WithRequest(c, log.Fields{"handler": "finalizeResult", "path": c.FullPath(), "matchID": matchID}))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "something went wrong"})
			c.Abort()
			return
		}
	}

	err = h.Service.ReportResult(c, matchID)
	if err != nil {
		if err == profixio.ErrAlreadyRegistered {
			log.Warning("request conflict", log.WithRequest(c, log.Fields{"handler": "finalizeResult", "path": c.FullPath(), "matchID": matchID, "step": "report", "reason": "already_registered"}))
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			c.Abort()
			return
		}
		log.Error("request failed", err, log.WithRequest(c, log.Fields{"handler": "finalizeResult", "path": c.FullPath(), "matchID": matchID, "step": "report"}))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "something went wrong"})
		c.Abort()
		return
	}

	log.Info("request completed", log.WithRequest(c, log.Fields{"handler": "finalizeResult", "path": c.FullPath(), "matchID": matchID}))
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

	c.Header("retry-after", strconv.Itoa(remainingSeconds))
	c.Header("x-retry-at", tooSoonErr.RetryAt.UTC().Format(time.RFC3339))
	c.Header("Access-Control-Expose-Headers", "x-retry-at, retry-after")
}
