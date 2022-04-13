package main

import (
	"flag"
	"github.com/r4start/go-url-shortener/internal/app"
	"net/http"
	"os"
)

type config struct {
	ServerAddress   string `env:"SERVER_ADDRESS" envDefault:":8080"`
	BaseURL         string `env:"BASE_URL"`
	FileStoragePath string `env:"FILE_STORAGE_PATH"`
}

func main() {
	cfg := config{}

	flag.StringVar(&cfg.ServerAddress, "a", os.Getenv("SERVER_ADDRESS"), "")
	flag.StringVar(&cfg.BaseURL, "b", os.Getenv("BASE_URL"), "")
	flag.StringVar(&cfg.FileStoragePath, "f", os.Getenv("FILE_STORAGE_PATH"), "")

	flag.Parse()

	if len(cfg.ServerAddress) == 0 {
		cfg.ServerAddress = ":8080"
	}

	handler, err := app.NewURLShortener(cfg.BaseURL, cfg.FileStoragePath)
	if err != nil {
		panic(err)
	}

	server := &http.Server{Addr: cfg.ServerAddress, Handler: handler}
	server.ListenAndServe()
}
