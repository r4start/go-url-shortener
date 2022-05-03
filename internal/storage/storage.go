package storage

import "context"

type UserData struct {
	ShortURLID  uint64
	OriginalURL string
}

type AddResult struct {
	ID       uint64
	Inserted bool
}

type URLStorage interface {
	Add(ctx context.Context, userID uint64, url string) (uint64, bool, error)
	AddURLs(ctx context.Context, userID uint64, urls []string) ([]AddResult, error)
	Get(ctx context.Context, id uint64) (string, error)
	GetUserData(ctx context.Context, userID uint64) ([]UserData, error)
	Close() error
}
