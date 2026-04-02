package errs

import (
	"fmt"
	"net/http"
)

func Write(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"status":%d,"detail":%q}`, status, msg)
}
