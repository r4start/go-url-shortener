package storage

type URLStorage interface {
	Add(url string) (uint64, bool, error)
	Get(id uint64) (string, error)
	Close() error
}
