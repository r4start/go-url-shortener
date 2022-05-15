package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"github.com/r4start/go-url-shortener/internal/app"
	"github.com/r4start/go-url-shortener/internal/storage"
	"net/http"
	"os"

	"go.uber.org/zap"

	_ "github.com/jackc/pgx/v4/stdlib"
)

type config struct {
	ServerAddress            string `env:"SERVER_ADDRESS" envDefault:":8080"`
	BaseURL                  string `env:"BASE_URL"`
	FileStoragePath          string `env:"FILE_STORAGE_PATH"`
	DatabaseConnectionString string `env:"DATABASE_DSN"`
}

func main() {
	cfg := config{}

	flag.StringVar(&cfg.ServerAddress, "a", os.Getenv("SERVER_ADDRESS"), "")
	flag.StringVar(&cfg.BaseURL, "b", os.Getenv("BASE_URL"), "")
	flag.StringVar(&cfg.FileStoragePath, "f", os.Getenv("FILE_STORAGE_PATH"), "")
	flag.StringVar(&cfg.DatabaseConnectionString, "d", os.Getenv("DATABASE_DSN"), "")

	flag.Parse()

	logger, err := zap.NewProduction()
	if err != nil {
		fmt.Printf("failed to initialize logger: %+v", err)
		os.Exit(1)
	}
	defer logger.Sync()

	if len(cfg.ServerAddress) == 0 {
		cfg.ServerAddress = ":8080"
	}

	storageContext, cancel := context.WithCancel(context.Background())
	defer cancel()

	st, dbConn, err := createStorage(storageContext, &cfg)
	if err != nil {
		logger.Fatal("failed to create a storage", zap.Error(err))
	}

	defer st.Close()

	serverContext, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler, err := app.NewURLShortener(serverContext, dbConn, cfg.BaseURL, st, logger)
	if err != nil {
		logger.Fatal("failed to create a storage", zap.Error(err))
	}

	server := &http.Server{Addr: cfg.ServerAddress, Handler: handler}
	server.ListenAndServe()
}

func createStorage(ctx context.Context, cfg *config) (storage.URLStorage, *sql.DB, error) {
	var st storage.URLStorage = nil
	var dbConn *sql.DB = nil
	var err error = nil

	if len(cfg.DatabaseConnectionString) != 0 {
		dbConn, err = sql.Open("pgx", cfg.DatabaseConnectionString)
		if err == nil {
			st, err = storage.NewDatabaseStorage(ctx, dbConn)
		}
	} else if len(cfg.FileStoragePath) != 0 {
		st, err = storage.NewFileStorage(cfg.FileStoragePath)
	} else {
		st = storage.NewInMemoryStorage()
	}

	return st, dbConn, err
}
