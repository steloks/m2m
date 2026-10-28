package api

import (
	"crypto/subtle"
	"net/http"
)

const tokenHeader = "X-Device-Token"

func requireToken(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get(tokenHeader)
		if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			writeError(w, http.StatusUnauthorized, "missing or invalid "+tokenHeader)
			return
		}
		next.ServeHTTP(w, r)
	})
}
