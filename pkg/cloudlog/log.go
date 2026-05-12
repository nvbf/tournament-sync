package cloudlog

import (
	"encoding/json"
	"fmt"
	stdlog "log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gin-gonic/gin"
)

type Fields map[string]any

func Printf(format string, v ...any) {
	msg := fmt.Sprintf(format, v...)
	logStructured(inferSeverity(msg), msg, nil)
}

func Println(v ...any) {
	msg := fmt.Sprintln(v...)
	logStructured(inferSeverity(msg), strings.TrimSpace(msg), nil)
}

func Print(v ...any) {
	msg := fmt.Sprint(v...)
	logStructured(inferSeverity(msg), msg, nil)
}

func Info(message string, fields Fields) {
	logStructured("INFO", message, fields)
}

func Warning(message string, fields Fields) {
	logStructured("WARNING", message, fields)
}

func Error(message string, err error, fields Fields) {
	enriched := cloneFields(fields)
	if err != nil {
		enriched["error"] = err.Error()
	}
	logStructured("ERROR", message, enriched)
}

func WithRequest(c *gin.Context, fields Fields) Fields {
	enriched := cloneFields(fields)
	if c == nil {
		return enriched
	}

	enriched["httpRequest"] = map[string]any{
		"requestMethod": c.Request.Method,
		"requestUrl":    c.Request.URL.String(),
		"userAgent":     c.Request.UserAgent(),
		"remoteIp":      c.ClientIP(),
	}

	traceID, spanID, sampled, ok := parseTraceContext(c.GetHeader("X-Cloud-Trace-Context"))
	if ok {
		projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
		if projectID == "" {
			projectID = os.Getenv("GCP_PROJECT")
		}
		if projectID != "" {
			enriched["logging.googleapis.com/trace"] = fmt.Sprintf("projects/%s/traces/%s", projectID, traceID)
		}
		if spanID != "" {
			enriched["logging.googleapis.com/spanId"] = spanID
		}
		enriched["logging.googleapis.com/trace_sampled"] = sampled
	}

	return enriched
}

func Fatalf(format string, v ...any) {
	msg := fmt.Sprintf(format, v...)
	logStructured("CRITICAL", msg, nil)
	os.Exit(1)
}

func Fatal(v ...any) {
	msg := fmt.Sprint(v...)
	logStructured("CRITICAL", msg, nil)
	os.Exit(1)
}

func Fatalln(v ...any) {
	msg := fmt.Sprintln(v...)
	logStructured("CRITICAL", strings.TrimSpace(msg), nil)
	os.Exit(1)
}

func logStructured(severity string, message string, fields Fields) {
	entry := map[string]any{
		"severity":  severity,
		"message":   message,
		"component": inferComponent(),
	}
	for k, v := range fields {
		entry[k] = v
	}

	payload, err := json.Marshal(entry)
	if err != nil {
		stdlog.Printf("severity=%s message=%s marshal_error=%v", severity, message, err)
		return
	}

	stdlog.Println(string(payload))
}

func cloneFields(fields Fields) Fields {
	if fields == nil {
		return Fields{}
	}
	cloned := make(Fields, len(fields))
	for k, v := range fields {
		cloned[k] = v
	}
	return cloned
}

func parseTraceContext(header string) (traceID string, spanID string, sampled bool, ok bool) {
	if header == "" {
		return "", "", false, false
	}

	parts := strings.Split(header, "/")
	if len(parts) == 0 || parts[0] == "" {
		return "", "", false, false
	}
	traceID = strings.TrimSpace(parts[0])

	if len(parts) > 1 {
		spanAndOpts := strings.Split(parts[1], ";")
		spanID = strings.TrimSpace(spanAndOpts[0])
		for _, opt := range spanAndOpts[1:] {
			opt = strings.TrimSpace(opt)
			if opt == "o=1" {
				sampled = true
			}
		}
	}

	return traceID, spanID, sampled, true
}

func inferSeverity(msg string) string {
	m := strings.ToLower(msg)
	if strings.Contains(m, " failed") || strings.Contains(m, "error") || strings.Contains(m, "could not") || strings.Contains(m, "panic") {
		return "ERROR"
	}
	if strings.Contains(m, "warn") || strings.Contains(m, "invalid") {
		return "WARNING"
	}
	return "INFO"
}

func inferComponent() string {
	for depth := 2; depth < 10; depth++ {
		_, file, _, ok := runtime.Caller(depth)
		if !ok {
			continue
		}
		if strings.Contains(file, "/pkg/cloudlog/") {
			continue
		}
		parts := strings.Split(filepath.ToSlash(file), "/")
		for i := 0; i < len(parts)-1; i++ {
			if parts[i] == "services" || parts[i] == "repos" || parts[i] == "pkg" || parts[i] == "cmd" {
				return parts[i+1]
			}
		}
		return strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	}
	return "app"
}
