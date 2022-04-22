package storage

import "context"

type UserData struct {
	ShortURLID  uint64
	OriginalURL string
}

type URLStorage interface {
	Add(ctx context.Context, userID uint64, url string) (uint64, bool, error)
	Get(ctx context.Context, id uint64) (string, error)
	GetUserData(ctx context.Context, userID uint64) ([]UserData, error)
	Close() error
}
