package main

import (
	"github.com/r4start/go-url-shortener/internal/app"

	"net/http"
)

func main() {
	handler := app.NewURLShortener()
	server := &http.Server{Addr: ":8080", Handler: handler}
	server.ListenAndServe()
}
