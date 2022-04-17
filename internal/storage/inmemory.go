package storage

import (
	"errors"
	"hash/fnv"
	"sync"
)

type syncMapStorage struct {
	urls map[uint64]string
	lock sync.RWMutex
}

func NewInMemoryStorage() URLStorage {
	return &syncMapStorage{
		urls: make(map[uint64]string),
		lock: sync.RWMutex{},
	}
}

func (s *syncMapStorage) Add(url string) (uint64, bool, error) {
	key, err := generateKey(&url)
	if err != nil {
		return 0, false, err
	}

	s.lock.Lock()
	defer s.lock.Unlock()
	if _, ok := s.urls[key]; ok {
		return key, ok, nil
	}

	s.urls[key] = url

	return key, false, nil
}

func (s *syncMapStorage) Get(id uint64) (string, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	if v, ok := s.urls[id]; ok {
		return v, nil
	}
	return "", errors.New("not found")
}

func (s *syncMapStorage) Close() error {
	return nil
}

type syncUserStorage struct {
	urls     map[uint64]string
	userData map[uint64][]UserData
	lock     sync.RWMutex
}

func NewInMemoryUserStorage() UserURLStorage {
	return &syncUserStorage{
		urls:     make(map[uint64]string),
		userData: make(map[uint64][]UserData),
		lock:     sync.RWMutex{},
	}
}

func (s *syncUserStorage) Add(userId uint64, url string) (uint64, bool, error) {
	key, err := generateKey(&url)
	if err != nil {
		return 0, false, err
	}

	s.lock.Lock()
	defer s.lock.Unlock()
	if _, ok := s.urls[key]; ok {
		return key, ok, nil
	}

	s.urls[key] = url
	data, _ := s.userData[userId]
	if len(data) == 0 {
		data = make([]UserData, 0)
	}

	data = append(data, UserData{
		ShortURLID:  key,
		OriginalURL: url,
	})

	s.userData[userId] = data

	return key, false, nil
}

func (s *syncUserStorage) Get(id uint64) (string, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	if v, ok := s.urls[id]; ok {
		return v, nil
	}
	return "", errors.New("not found")
}

func (s *syncUserStorage) GetUserData(userId uint64) ([]UserData, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	if v, ok := s.userData[userId]; ok {
		return v, nil
	}
	return nil, errors.New("not found")
}

func (s *syncUserStorage) Close() error {
	return nil
}

func generateKey(url *string) (uint64, error) {
	hasher := fnv.New64()
	_, err := hasher.Write([]byte(*url))
	if err != nil {
		return 0, err
	}
	return hasher.Sum64(), nil
}
