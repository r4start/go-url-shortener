package main

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	_ "github.com/jackc/pgx/v4/stdlib"

	"github.com/r4start/go-url-shortener/internal/app"
	"github.com/r4start/go-url-shortener/pkg/storage"
)

//go:generate sh -c "git branch --show-current > branch.txt"
//go:generate sh -c "printf %s $(git rev-parse HEAD) > commit.txt"
//go:generate sh -c "sh -c 'date +%Y-%m-%dT%H:%M:%S' > date.txt"

//go:embed *
var buildInfo embed.FS

//go:embed key.pem
var serverKey []byte

//go:embed cert.pem
var serverCert []byte

const (
	serverKeyFileName  = "key.pem"
	serverCertFileName = "cert.pem"
)

type config struct {
	ServerAddress            string `json:"server_address" env:"SERVER_ADDRESS" envDefault:":8080"`
	BaseURL                  string `json:"base_url" env:"BASE_URL"`
	FileStoragePath          string `json:"file_storage_path" env:"FILE_STORAGE_PATH"`
	DatabaseConnectionString string `json:"database_dsn" env:"DATABASE_DSN"`
	ServeTLS                 bool   `json:"enable_https" env:"ENABLE_HTTPS"`
	configFile               string
}

func main() {
	printStartupMessage()

	cfg := config{}

	flag.StringVar(&cfg.ServerAddress, "a", os.Getenv("SERVER_ADDRESS"), "")
	flag.StringVar(&cfg.BaseURL, "b", os.Getenv("BASE_URL"), "")
	flag.StringVar(&cfg.FileStoragePath, "f", os.Getenv("FILE_STORAGE_PATH"), "")
	flag.StringVar(&cfg.DatabaseConnectionString, "d", os.Getenv("DATABASE_DSN"), "")
	flag.StringVar(&cfg.configFile, "c", os.Getenv("CONFIG"), "")

	if _, exists := os.LookupEnv("ENABLE_HTTPS"); exists {
		flag.BoolVar(&cfg.ServeTLS, "s", true, "")
	} else {
		flag.BoolVar(&cfg.ServeTLS, "s", false, "")
	}

	flag.Parse()

	logger, err := zap.NewProduction()
	if err != nil {
		fmt.Printf("failed to initialize logger: %+v", err)
		return
	}
	defer func() {
		if err := logger.Sync(); err != nil {
			fmt.Println(err)
		}
	}()

	if len(cfg.configFile) != 0 {
		c, err := loadConfigFromFile(cfg.configFile)
		if err != nil {
			logger.Fatal("failed to load config file", zap.Error(err), zap.String("path", cfg.configFile))
		}
		cfg = *c
	}

	if len(cfg.ServerAddress) == 0 {
		cfg.ServerAddress = ":8080"
	}

	storageContext, cancel := context.WithCancel(context.Background())
	defer cancel()

	st, dbConn, err := createStorage(storageContext, &cfg)
	if err != nil {
		logger.Fatal("failed to create a storage", zap.Error(err))
	}

	defer func() {
		if err := st.Close(); err != nil {
			fmt.Println(err)
		}
	}()

	serverContext, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler, err := app.NewURLShortener(serverContext, dbConn, cfg.BaseURL, st, logger)
	if err != nil {
		logger.Fatal("failed to create a storage", zap.Error(err))
	}

	server := &http.Server{Addr: cfg.ServerAddress, Handler: handler}

	sCh, err := prepareShutdown(server, logger)
	if err != nil {
		logger.Fatal("failed to prepare shutdown", zap.Error(err))
	}

	if cfg.ServeTLS {
		if err := prepareTLSFile(serverKeyFileName, serverKey); err != nil {
			logger.Fatal("failed to create a key file", zap.Error(err))
		}
		if err := prepareTLSFile(serverCertFileName, serverCert); err != nil {
			logger.Fatal("failed to create a cert file", zap.Error(err))
		}
		err = server.ListenAndServeTLS(serverCertFileName, serverKeyFileName)
	} else {
		err = server.ListenAndServe()
	}

	<-sCh

	fmt.Println("Server shutdown")
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

func printStartupMessage() {
	buildVersion := "N/A\n"
	buildDate := buildVersion
	buildCommit := buildVersion

	if data, err := buildInfo.ReadFile("branch.txt"); err == nil {
		buildVersion = string(data)
	}

	if data, err := buildInfo.ReadFile("commit.txt"); err == nil {
		buildCommit = string(data)
	}

	if data, err := buildInfo.ReadFile("date.txt"); err == nil {
		buildDate = string(data)
	}

	fmt.Printf("Build version: %s", buildVersion)
	fmt.Printf("Build date: %s", buildDate)
	fmt.Printf("Build commit: %s\n\n", buildCommit)
}

func prepareTLSFile(name string, data []byte) error {
	file, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.Write(data)
	return err
}

func loadConfigFromFile(filePath string) (*config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var result config
	err = json.Unmarshal(data, &result)
	return &result, err
}

func prepareShutdown(server *http.Server, logger *zap.Logger) (<-chan any, error) {
	shutdownSig := make(chan any)
	signals := make(chan os.Signal, 1)

	signal.Notify(signals, syscall.SIGINT)
	signal.Notify(signals, syscall.SIGTERM)
	signal.Notify(signals, syscall.SIGQUIT)

	go func() {
		<-signals

		if err := server.Shutdown(context.Background()); err != nil {
			logger.Error("failed to shutdown a server", zap.Error(err))
		}

		close(shutdownSig)
	}()

	return shutdownSig, nil
}
