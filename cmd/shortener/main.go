package main

import (
	"fmt"
	"github.com/caarlos0/env/v6"
	"github.com/r4start/go-url-shortener/internal/app"

	"net/http"
)

type config struct {
	ServerAddress string `env:"SERVER_ADDRESS" envDefault:":8080"`
	BaseURL       string `env:"BASE_URL"`
}

func main() {
	cfg := config{}

	if err := env.Parse(&cfg); err != nil {
		fmt.Printf("%+v\n", err)
		return
	}

	handler := app.NewURLShortener(cfg.BaseURL)
	server := &http.Server{Addr: cfg.ServerAddress, Handler: handler}
	server.ListenAndServe()
}
