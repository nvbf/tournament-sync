package matches

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	profixio "github.com/nvbf/tournament-sync/repos/profixio"
)

type testResultsService struct {
	reportErr           error
	finalizeErr         error
	reportCalled        int
	finalizeCalled      int
	lastReportMatchID   string
	lastFinalizeMatchID string
}

func (s *testResultsService) ReportResult(_ *gin.Context, matchID string) error {
	s.reportCalled++
	s.lastReportMatchID = matchID
	return s.reportErr
}

func (s *testResultsService) FinalizeResult(_ *gin.Context, matchID string) error {
	s.finalizeCalled++
	s.lastFinalizeMatchID = matchID
	return s.finalizeErr
}

func setupMatchesRouter(service Results) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	NewHTTPHandler(HTTPOptions{Service: service, Router: r})
	return r
}

func performRequest(r *gin.Engine, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func responseMessage(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response body: %v", err)
	}

	if msg, ok := body["message"]; ok {
		return msg
	}
	return body["error"]
}

func TestResultHandler(t *testing.T) {
	cases := []struct {
		name            string
		reportErr       error
		expectedStatus  int
		expectedMessage string
	}{
		{
			name:            "result registered",
			reportErr:       nil,
			expectedStatus:  http.StatusAccepted,
			expectedMessage: "Result registered",
		},
		{
			name:            "already registered",
			reportErr:       profixio.ErrAlreadyRegistered,
			expectedStatus:  http.StatusConflict,
			expectedMessage: profixio.ErrAlreadyRegistered.Error(),
		},
		{
			name:            "internal error",
			reportErr:       errors.New("boom"),
			expectedStatus:  http.StatusInternalServerError,
			expectedMessage: "something went wrong",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			service := &testResultsService{reportErr: c.reportErr}
			r := setupMatchesRouter(service)

			w := performRequest(r, http.MethodGet, "/result/match-42")

			if w.Code != c.expectedStatus {
				t.Fatalf("expected status %d, got %d", c.expectedStatus, w.Code)
			}
			if responseMessage(t, w) != c.expectedMessage {
				t.Fatalf("expected message %q, got %q", c.expectedMessage, responseMessage(t, w))
			}
			if service.reportCalled != 1 {
				t.Fatalf("expected report to be called once, got %d", service.reportCalled)
			}
			if service.lastReportMatchID != "match-42" {
				t.Fatalf("expected report match ID match-42, got %s", service.lastReportMatchID)
			}
		})
	}
}

func TestFinalizeResultHandler(t *testing.T) {
	cases := []struct {
		name                string
		finalizeErr         error
		reportErr           error
		expectedStatus      int
		expectedMessage     string
		expectedReportCalls int
		expectedRetryAfter  string
		expectRetryAtHeader bool
	}{
		{
			name:                "finalize and report success",
			finalizeErr:         nil,
			reportErr:           nil,
			expectedStatus:      http.StatusAccepted,
			expectedMessage:     "Result finalized and registered",
			expectedReportCalls: 1,
			expectedRetryAfter:  "",
			expectRetryAtHeader: false,
		},
		{
			name:                "finalize too soon",
			finalizeErr:         &FinalizeTooSoonError{RetryAt: time.Now().Add(2 * time.Minute)},
			reportErr:           nil,
			expectedStatus:      http.StatusBadRequest,
			expectedMessage:     ErrFinalizeTooSoon.Error(),
			expectedReportCalls: 0,
			expectedRetryAfter:  "",
			expectRetryAtHeader: true,
		},
		{
			name:                "invalid match result",
			finalizeErr:         ErrInvalidMatchResult,
			reportErr:           nil,
			expectedStatus:      http.StatusBadRequest,
			expectedMessage:     ErrInvalidMatchResult.Error(),
			expectedReportCalls: 0,
			expectedRetryAfter:  "",
			expectRetryAtHeader: false,
		},
		{
			name:                "no events to finalize",
			finalizeErr:         ErrNoEventsToFinalize,
			reportErr:           nil,
			expectedStatus:      http.StatusBadRequest,
			expectedMessage:     ErrNoEventsToFinalize.Error(),
			expectedReportCalls: 0,
			expectedRetryAfter:  "",
			expectRetryAtHeader: false,
		},
		{
			name:                "already finalized",
			finalizeErr:         ErrMatchAlreadyFinalized,
			reportErr:           nil,
			expectedStatus:      http.StatusConflict,
			expectedMessage:     ErrMatchAlreadyFinalized.Error(),
			expectedReportCalls: 0,
			expectedRetryAfter:  "",
			expectRetryAtHeader: false,
		},
		{
			name:                "finalize internal error",
			finalizeErr:         errors.New("finalize failed"),
			reportErr:           nil,
			expectedStatus:      http.StatusInternalServerError,
			expectedMessage:     "something went wrong",
			expectedReportCalls: 0,
			expectedRetryAfter:  "",
			expectRetryAtHeader: false,
		},
		{
			name:                "report already registered",
			finalizeErr:         nil,
			reportErr:           profixio.ErrAlreadyRegistered,
			expectedStatus:      http.StatusConflict,
			expectedMessage:     profixio.ErrAlreadyRegistered.Error(),
			expectedReportCalls: 1,
			expectedRetryAfter:  "",
			expectRetryAtHeader: false,
		},
		{
			name:                "report internal error",
			finalizeErr:         nil,
			reportErr:           errors.New("report failed"),
			expectedStatus:      http.StatusInternalServerError,
			expectedMessage:     "something went wrong",
			expectedReportCalls: 1,
			expectedRetryAfter:  "",
			expectRetryAtHeader: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			service := &testResultsService{finalizeErr: c.finalizeErr, reportErr: c.reportErr}
			r := setupMatchesRouter(service)

			w := performRequest(r, http.MethodPut, "/result/finalize/match-99")

			if w.Code != c.expectedStatus {
				t.Fatalf("expected status %d, got %d", c.expectedStatus, w.Code)
			}
			if responseMessage(t, w) != c.expectedMessage {
				t.Fatalf("expected message %q, got %q", c.expectedMessage, responseMessage(t, w))
			}
			if service.finalizeCalled != 1 {
				t.Fatalf("expected finalize to be called once, got %d", service.finalizeCalled)
			}
			if service.lastFinalizeMatchID != "match-99" {
				t.Fatalf("expected finalize match ID match-99, got %s", service.lastFinalizeMatchID)
			}
			if service.reportCalled != c.expectedReportCalls {
				t.Fatalf("expected report calls %d, got %d", c.expectedReportCalls, service.reportCalled)
			}
			if c.expectedReportCalls > 0 && service.lastReportMatchID != "match-99" {
				t.Fatalf("expected report match ID match-99, got %s", service.lastReportMatchID)
			}

			retryAfter := w.Header().Get("Retry-After")
			retryAt := w.Header().Get("X-Retry-At")
			if c.expectRetryAtHeader {
				if retryAfter == "" {
					t.Fatalf("expected Retry-After header to be set")
				}
				if _, err := strconv.Atoi(retryAfter); err != nil {
					t.Fatalf("expected Retry-After to be an integer, got %q", retryAfter)
				}
				if _, err := time.Parse(time.RFC3339, retryAt); err != nil {
					t.Fatalf("expected X-Retry-At to be RFC3339, got %q", retryAt)
				}
			} else {
				if retryAfter != c.expectedRetryAfter {
					t.Fatalf("expected Retry-After %q, got %q", c.expectedRetryAfter, retryAfter)
				}
				if retryAt != "" {
					t.Fatalf("expected X-Retry-At to be empty, got %q", retryAt)
				}
			}
		})
	}
}
