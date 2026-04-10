package transport

import (
	"log"
	"net/http"
	"time"
)

type responseCapture struct {
	http.ResponseWriter
	status int
}

func (w *responseCapture) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// LoggingMiddleware registra método, ruta, código de respuesta y duración (útil para depurar).
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rc := &responseCapture{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rc, r)
		log.Printf("%s %s -> %d (%s)", r.Method, r.URL.RequestURI(), rc.status, time.Since(start).Truncate(time.Millisecond))
	})
}
