package middleware

import (
	"log/slog"
	"net/http"
)

const (
	corsAllowMethods = "GET, POST, PUT, DELETE, OPTIONS"

	corsAllowHeaders = "Accept, Authorization, Content-Type, Connect-Protocol-Version, Connect-Timeout-Ms"

	corsExposeHeaders = "Connect-Content-Encoding, Connect-Protocol-Version"

	corsMaxAge = "86400"
)

func CORS(allowedOrigins []string, allowAll bool, logger *slog.Logger) func(http.Handler) http.Handler {
	originSet := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		originSet[o] = true
	}

	if allowAll {
		logger.Warn("CORS: allow-all mode enabled — do not use in production")
	} else if len(allowedOrigins) == 0 {
		logger.Warn("CORS: no origins configured (CONTROL_CORS_ORIGINS), all cross-origin requests will be denied")
	} else {
		logger.Info("CORS: allowed origins configured", "origins", allowedOrigins)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			if origin != "" {
				switch {
				case originSet[origin]:

					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				case allowAll:

					w.Header().Set("Access-Control-Allow-Origin", origin)
				default:

					if r.Method == http.MethodOptions {
						w.WriteHeader(http.StatusForbidden)
						return
					}
					next.ServeHTTP(w, r)
					return
				}
			}

			if r.Method == http.MethodOptions {
				w.Header().Add("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Methods", corsAllowMethods)
				w.Header().Set("Access-Control-Allow-Headers", corsAllowHeaders)
				w.Header().Set("Access-Control-Expose-Headers", corsExposeHeaders)
				w.Header().Set("Access-Control-Max-Age", corsMaxAge)
				w.Header().Add("Vary", "Access-Control-Request-Method")
				w.Header().Add("Vary", "Access-Control-Request-Headers")
				w.WriteHeader(http.StatusNoContent)
				return
			}

			w.Header().Add("Vary", "Origin")

			w.Header().Set("Access-Control-Expose-Headers", corsExposeHeaders)

			next.ServeHTTP(w, r)
		})
	}
}
