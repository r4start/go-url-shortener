package storage

type URLStorage interface {
	Add(url string) (uint64, error)
	Get(id uint64) (string, error)
}
