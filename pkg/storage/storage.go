// Package storage provides high level interface of URL storage operations.
package storage

import (
	"context"
	"errors"
)

var (
	// ErrDeleted - an entry has been deleted.
	ErrDeleted = errors.New("deleted")
	// ErrNotFound - an entry hasn't been found.
	ErrNotFound = errors.New("not found")
)

// UserData information about users shortened URLs.
type UserData struct {
	// ShortURLID - a storage ID for a short URL. An actual short URL can be retrieved with the id and Get method.
	ShortURLID uint64
	// OriginalURL - a user provided URL.
	OriginalURL string
}

// AddResult result of Add operation.
type AddResult struct {
	// ID - short URL id.
	ID uint64
	// Inserted - true iff URL was actually inserted.
	Inserted bool
}

// URLStorage - interface that every storage has to implement.
type URLStorage interface {
	// Add - add an url for a userID.
	Add(ctx context.Context, userID uint64, url string) (uint64, bool, error)
	// AddURLs - batch urls add.
	AddURLs(ctx context.Context, userID uint64, urls []string) ([]AddResult, error)
	// DeleteURLs - batch urls delete.
	DeleteURLs(ctx context.Context, userID uint64, ids []uint64) error
	// Get - get original URL for an id.
	Get(ctx context.Context, id uint64) (string, error)
	// GetUserData - get all user shortened URLs.
	GetUserData(ctx context.Context, userID uint64) ([]UserData, error)
	// Close - close URLStorage. Should be called in the end of lifetime.
	// An implementation may cleanup necessary resources here.
	Close() error
}
