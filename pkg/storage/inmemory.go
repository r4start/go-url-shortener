package storage

import (
	"context"
	"sync"
)

var (
	_ URLStorage  = (*syncMapStorage)(nil)
	_ ServiceStat = (*syncMapStorage)(nil)
)

type syncMapStorage struct {
	urls     map[uint64]string
	userData map[uint64][]UserData
	goneIds  map[uint64]bool
	lock     sync.RWMutex
}

// NewInMemoryStorage creates URLStorage implementation that doesn't have any persistent storage.
func NewInMemoryStorage() *syncMapStorage {
	return &syncMapStorage{
		urls:     make(map[uint64]string),
		userData: make(map[uint64][]UserData),
		goneIds:  make(map[uint64]bool),
		lock:     sync.RWMutex{},
	}
}

func (s *syncMapStorage) Add(ctx context.Context, userID uint64, url string) (uint64, bool, error) {
	key, err := generateKey(url)
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
		key, err := generateKey(url)
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
	idsToDelete := make(map[uint64]bool)
	userIDs := make(map[uint64]bool)

	s.lock.RLock()
	// As a first step we need to sort out whether an id is in our storage.
	// We don't want to mark missed keys as deleted.
	// Moreover, we need to check whether an id is owned by a particular user.
	userData, actualUser := s.userData[userID]
	if !actualUser {
		s.lock.RUnlock()
		return ErrNotFound
	}

	for _, e := range userData {
		userIDs[e.ShortURLID] = true
	}
	s.lock.RUnlock()

	for _, id := range ids {
		if _, ok := userIDs[id]; ok {
			idsToDelete[id] = true
		}
	}

	if len(idsToDelete) == 0 {
		return nil
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	for id, _ := range idsToDelete {
		s.goneIds[id] = true
	}

	userData = s.userData[userID]
	s.userData[userID] = make([]UserData, 0)
	for _, d := range userData {
		if idsToDelete[d.ShortURLID] {
			continue
		}
		s.userData[userID] = append(s.userData[userID], d)
	}

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

func (s *syncMapStorage) TotalUsers(context.Context) (uint64, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return uint64(len(s.userData)), nil
}

func (s *syncMapStorage) TotalURLs(context.Context) (uint64, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return uint64(len(s.urls) - len(s.goneIds)), nil
}
