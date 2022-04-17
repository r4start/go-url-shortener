package storage

type URLStorage interface {
	Add(url string) (uint64, bool, error)
	Get(id uint64) (string, error)
	Close() error
}

type UserData struct {
	ShortURLID  uint64
	OriginalURL string
}

type UserURLStorage interface {
	Add(userId uint64, url string) (uint64, bool, error)
	Get(id uint64) (string, error)
	GetUserData(userId uint64) ([]UserData, error)
	Close() error
}
