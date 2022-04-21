package storage

type UserData struct {
	ShortURLID  uint64
	OriginalURL string
}

type URLStorage interface {
	Add(userID uint64, url string) (uint64, bool, error)
	Get(id uint64) (string, error)
	GetUserData(userID uint64) ([]UserData, error)
	Close() error
}
