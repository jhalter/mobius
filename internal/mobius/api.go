package mobius

import (
	"bytes"
	"encoding/json"
	"github.com/jhalter/mobius/hotline"
	"io"
	"log"
	"log/slog"
	"net/http"
)

type logResponseWriter struct {
	http.ResponseWriter
	statusCode int
	buf        bytes.Buffer
}

func NewLogResponseWriter(w http.ResponseWriter) *logResponseWriter {
	return &logResponseWriter{w, http.StatusOK, bytes.Buffer{}}
}

func (lrw *logResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

func (lrw *logResponseWriter) Write(b []byte) (int, error) {
	lrw.buf.Write(b)
	return lrw.ResponseWriter.Write(b)
}

type APIServer struct {
	hlServer *hotline.Server
	logger   *slog.Logger
	mux      *http.ServeMux
}

func (srv *APIServer) logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lrw := NewLogResponseWriter(w)
		next.ServeHTTP(lrw, r)

		srv.logger.Info("req", "method", r.Method, "url", r.URL.Path, "remoteAddr", r.RemoteAddr, "response_code", lrw.statusCode)
	})
}

func NewAPIServer(hlServer *hotline.Server, reloadFunc func(), logger *slog.Logger) *APIServer {
	srv := APIServer{
		hlServer: hlServer,
		logger:   logger,
		mux:      http.NewServeMux(),
	}

	srv.mux.Handle("/api/v1/reload", srv.logMiddleware(http.HandlerFunc(srv.ReloadHandler(reloadFunc))))
	srv.mux.Handle("/api/v1/shutdown", srv.logMiddleware(http.HandlerFunc(srv.ShutdownHandler)))
	srv.mux.Handle("/api/v1/stats", srv.logMiddleware(http.HandlerFunc(srv.RenderStats)))

	return &srv
}

func (srv *APIServer) ShutdownHandler(w http.ResponseWriter, r *http.Request) {
	msg, err := io.ReadAll(r.Body)
	if err != nil || len(msg) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	go srv.hlServer.Shutdown(msg)

	_, _ = io.WriteString(w, `{ "msg": "server shutting down" }`)
}

func (srv *APIServer) ReloadHandler(reloadFunc func()) func(w http.ResponseWriter, _ *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		reloadFunc()

		_, _ = io.WriteString(w, `{ "msg": "config reloaded" }`)
	}
}

func (srv *APIServer) RenderStats(w http.ResponseWriter, _ *http.Request) {
	u, err := json.Marshal(srv.hlServer.CurrentStats())
	if err != nil {
		panic(err)
	}

	_, _ = io.WriteString(w, string(u))
}

func (srv *APIServer) Serve(port string) {
	err := http.ListenAndServe(port, srv.mux)
	if err != nil {
		log.Fatal(err)
	}
}
