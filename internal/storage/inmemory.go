package storage

import (
	"context"
	"sync"
)

type syncMapStorage struct {
	urls     map[uint64]string
	userData map[uint64][]UserData
	goneIds  map[uint64]bool
	lock     sync.RWMutex
}

func NewInMemoryStorage() URLStorage {
	return &syncMapStorage{
		urls:     make(map[uint64]string),
		userData: make(map[uint64][]UserData),
		goneIds:  make(map[uint64]bool),
		lock:     sync.RWMutex{},
	}
}

func (s *syncMapStorage) Add(ctx context.Context, userID uint64, url string) (uint64, bool, error) {
	key, err := generateKey(&url)
	if err != nil {
		return 0, false, err
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	delete(s.goneIds, key)

	if _, ok := s.urls[key]; ok {
		return key, ok, nil
	}

	s.urls[key] = url
	data := s.userData[userID]
	if len(data) == 0 {
		data = make([]UserData, 0)
	}

	data = append(data, UserData{
		ShortURLID:  key,
		OriginalURL: url,
	})

	s.userData[userID] = data

	return key, false, nil
}

func (s *syncMapStorage) AddURLs(ctx context.Context, userID uint64, urls []string) ([]AddResult, error) {
	keys := make([]uint64, 0)
	for _, url := range urls {
		key, err := generateKey(&url)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}

	result := make([]AddResult, 0)

	s.lock.Lock()
	defer s.lock.Unlock()

	for i, url := range urls {
		delete(s.goneIds, keys[i])

		if _, ok := s.urls[keys[i]]; ok {
			result = append(result, AddResult{
				ID:       keys[i],
				Inserted: false,
			})
			continue
		}

		s.urls[keys[i]] = url
		data := s.userData[userID]
		if len(data) == 0 {
			data = make([]UserData, 0)
		}

		data = append(data, UserData{
			ShortURLID:  keys[i],
			OriginalURL: url,
		})

		s.userData[userID] = data

		result = append(result, AddResult{
			ID:       keys[i],
			Inserted: true,
		})
	}

	return result, nil
}

func (s *syncMapStorage) DeleteURLs(ctx context.Context, userID uint64, ids []uint64) error {
	return nil
}

func (s *syncMapStorage) Get(ctx context.Context, id uint64) (string, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	if _, ok := s.goneIds[id]; ok {
		return "", ErrDeleted
	}

	if v, ok := s.urls[id]; ok {
		return v, nil
	}
	return "", ErrNotFound
}

func (s *syncMapStorage) GetUserData(ctx context.Context, userID uint64) ([]UserData, error) {
	result := make([]UserData, 0)

	s.lock.RLock()
	defer s.lock.RUnlock()

	if _, ok := s.userData[userID]; !ok {
		return nil, ErrNotFound
	}

	for _, v := range s.userData[userID] {
		if _, ok := s.goneIds[v.ShortURLID]; ok {
			continue
		}
		result = append(result, v)
	}

	return result, nil
}

func (s *syncMapStorage) Close() error {
	return nil
}
