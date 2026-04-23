package auth

import (
	"context"
	"net/http"
)

const sessionCookieName = "stronglifts_session"
const userContextKey = "user"

// GetSessionCookieName returns the session cookie name
func GetSessionCookieName() string {
	return sessionCookieName
}

// SessionMiddleware checks for a valid session and adds the user to context
func SessionMiddleware(store *SessionStore) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(sessionCookieName)
			if err != nil {
				http.Redirect(w, r, "/login", http.StatusFound)
				return
			}

			user, err := store.Get(cookie.Value)
			if err != nil {
				http.Error(w, "Failed to load session", http.StatusInternalServerError)
				return
			}
			if user == nil {
				http.Redirect(w, r, "/login", http.StatusFound)
				return
			}

			// Add user to request context
			ctx := context.WithValue(r.Context(), userContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserFromContext retrieves the user from request context
func UserFromContext(r *http.Request) *UserSession {
	user, ok := r.Context().Value(userContextKey).(*UserSession)
	if !ok {
		return nil
	}
	return user
}
