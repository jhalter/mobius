package mobius

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jhalter/mobius/hotline"
	"github.com/redis/go-redis/v9"
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
	apiKey   string
	redis    *redis.Client
}

func (srv *APIServer) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if srv.apiKey != "" && r.Header.Get("X-API-Key") != srv.apiKey {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (srv *APIServer) logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lrw := NewLogResponseWriter(w)
		next.ServeHTTP(lrw, r)
		srv.logger.Info("req", "method", r.Method, "url", r.URL.Path, "remoteAddr", r.RemoteAddr, "response_code", lrw.statusCode)
	})
}

func NewAPIServer(hlServer *hotline.Server, reloadFunc func(), logger *slog.Logger, apiKey string, redisAddr string, redisPassword string, redisDB int) *APIServer {
	srv := APIServer{
		hlServer: hlServer,
		logger:   logger,
		mux:      http.NewServeMux(),
		apiKey:   apiKey,
	}
	if redisAddr != "" {
		srv.redis = redis.NewClient(&redis.Options{
			Addr:     redisAddr,
			Password: redisPassword,
			DB:       redisDB,
		})
		hlServer.Redis = srv.redis
	}

	srv.mux.Handle("/api/v1/online", srv.logMiddleware(srv.authMiddleware(http.HandlerFunc(srv.OnlineHandler))))
	srv.mux.Handle("/api/v1/ban", srv.logMiddleware(srv.authMiddleware(http.HandlerFunc(srv.BanHandler))))
	srv.mux.Handle("/api/v1/unban", srv.logMiddleware(srv.authMiddleware(http.HandlerFunc(srv.UnbanHandler))))
	srv.mux.Handle("/api/v1/banned/ips", srv.logMiddleware(srv.authMiddleware(http.HandlerFunc(srv.ListBannedIPsHandler))))
	srv.mux.Handle("/api/v1/banned/usernames", srv.logMiddleware(srv.authMiddleware(http.HandlerFunc(srv.ListBannedUsernamesHandler))))
	srv.mux.Handle("/api/v1/banned/nicknames", srv.logMiddleware(srv.authMiddleware(http.HandlerFunc(srv.ListBannedNicknamesHandler))))
	srv.mux.Handle("/api/v1/reload", srv.logMiddleware(srv.authMiddleware(http.HandlerFunc(srv.ReloadHandler(reloadFunc)))))
	srv.mux.Handle("/api/v1/shutdown", srv.logMiddleware(srv.authMiddleware(http.HandlerFunc(srv.ShutdownHandler))))
	srv.mux.Handle("/api/v1/stats", srv.logMiddleware(srv.authMiddleware(http.HandlerFunc(srv.RenderStats))))

	if srv.redis != nil {
		if err := srv.redis.Del(context.Background(), "mobius:online").Err(); err != nil {
			srv.logger.Warn("Failed to clear mobius:online in Redis", "err", err)
		} else {
			srv.logger.Info("Cleared mobius:online in Redis on startup")
		}
	}

	return &srv
}

func (srv *APIServer) OnlineHandler(w http.ResponseWriter, r *http.Request) {
	var users []map[string]string

	if srv.redis != nil {
		members, err := srv.redis.SMembers(r.Context(), "mobius:online").Result()
		if err == nil {
			for _, m := range members {
				parts := strings.SplitN(m, ":", 3)
				if len(parts) == 3 {
					users = append(users, map[string]string{
						"login":    parts[0],
						"nickname": parts[1],
						"ip":       parts[2],
					})
				}
			}
		}
	} else {
		for _, c := range srv.hlServer.ClientMgr.List() {
			users = append(users, map[string]string{
				"login":    string(c.Account.Login),
				"nickname": string(c.UserName),
				"ip":       c.RemoteAddr,
			})
		}
	}

	json.NewEncoder(w).Encode(users)
}

type BanRequest struct {
	Username string `json:"username,omitempty"`
	Nickname string `json:"nickname,omitempty"`
	IP       string `json:"ip,omitempty"`
}

func (srv *APIServer) BanHandler(w http.ResponseWriter, r *http.Request) {
	var req BanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if req.Username == "" && req.Nickname == "" && req.IP == "" {
		http.Error(w, "username, nickname, or ip required", http.StatusBadRequest)
		return
	}

	if srv.redis != nil {
		if req.Username != "" {
			srv.redis.SAdd(r.Context(), "mobius:banned:users", req.Username)
		}
		if req.Nickname != "" {
			srv.redis.SAdd(r.Context(), "mobius:banned:nicknames", req.Nickname)
		}
		if req.IP != "" {
			srv.redis.SAdd(r.Context(), "mobius:banned:ips", req.IP)
		}
	} else {
		// TODO: Fallback
	}

	// Disconnect user if online
	for _, c := range srv.hlServer.ClientMgr.List() {
		if (req.Username != "" && string(c.Account.Login) == req.Username) ||
			(req.Nickname != "" && string(c.UserName) == req.Nickname) ||
			(req.IP != "" && c.RemoteAddr == req.IP) {
			c.Disconnect()
		}
	}

	w.Write([]byte(`{"msg":"banned"}`))
}

func (srv *APIServer) UnbanHandler(w http.ResponseWriter, r *http.Request) {
	var req BanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if req.Username == "" && req.Nickname == "" && req.IP == "" {
		http.Error(w, "username, nickname, or ip required", http.StatusBadRequest)
		return
	}

	if srv.redis != nil {
		if req.Username != "" {
			srv.redis.SRem(r.Context(), "mobius:banned:users", req.Username)
		}
		if req.Nickname != "" {
			srv.redis.SRem(r.Context(), "mobius:banned:nicknames", req.Nickname)
		}
		if req.IP != "" {
			srv.redis.SRem(r.Context(), "mobius:banned:ips", req.IP)
		}
	} else {
		// TODO: Fallback
	}

	w.Write([]byte(`{"msg":"unbanned"}`))
}

func (srv *APIServer) ListBannedIPsHandler(w http.ResponseWriter, r *http.Request) {
	if srv.redis != nil {
		ips, err := srv.redis.SMembers(r.Context(), "mobius:banned:ips").Result()
		if err != nil {
			http.Error(w, "failed to fetch banned IPs", http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(ips)
	} else {
		// TODO: Fallback
	}
}

func (srv *APIServer) ListBannedUsernamesHandler(w http.ResponseWriter, r *http.Request) {
	if srv.redis != nil {
		users, err := srv.redis.SMembers(r.Context(), "mobius:banned:users").Result()
		if err != nil {
			http.Error(w, "failed to fetch banned usernames", http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(users)
	} else {
		// TODO: Fallback
	}
}

func (srv *APIServer) ListBannedNicknamesHandler(w http.ResponseWriter, r *http.Request) {
	if srv.redis != nil {
		nicks, err := srv.redis.SMembers(r.Context(), "mobius:banned:nicknames").Result()
		if err != nil {
			http.Error(w, "failed to fetch banned nicknames", http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(nicks)
	} else {
		// TODO: Fallback
	}
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
		http.Error(w, "failed to marshal stats", http.StatusInternalServerError)
		return
	}

	_, _ = w.Write(u)
}

func (srv *APIServer) Serve(port string) {
	err := http.ListenAndServe(port, srv.mux)
	if err != nil {
		log.Fatal(err)
	}
}
