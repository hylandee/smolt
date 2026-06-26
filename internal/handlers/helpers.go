package handlers

import (
	"log/slog"
	"net/http"
)

func httpError(w http.ResponseWriter, err error, msg string, code int) {
	slog.Error(msg, "error", err)
	http.Error(w, msg, code)
}
