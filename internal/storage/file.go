package storage

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
)

type fileStorage struct {
	file          *os.File
	writer        *bufio.Writer
	fileLock      sync.Mutex
	memoryStorage URLStorage
}

func NewFileStorage(filePath string) (URLStorage, error) {
	file, err := os.OpenFile(filePath, os.O_RDWR|os.O_APPEND|os.O_CREATE|os.O_SYNC, 0644)
	if err != nil {
		return nil, err
	}

	storage := &fileStorage{
		file:          file,
		writer:        bufio.NewWriter(file),
		fileLock:      sync.Mutex{},
		memoryStorage: NewInMemoryStorage(),
	}

	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}

	if stat.Size() == 0 {
		return storage, nil
	}

	ctx := context.Background()
	decoder := json.NewDecoder(file)
	for {
		data := make(map[string]string)
		if err := decoder.Decode(&data); err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		for k, v := range data {
			userID, err := strconv.ParseUint(k, 10, 64)
			if err != nil {
				return nil, err
			}
			if _, _, err := storage.memoryStorage.Add(ctx, userID, v); err != nil {
				return nil, err
			}
		}
	}

	return storage, nil
}

func (s *fileStorage) Add(ctx context.Context, userID uint64, url string) (uint64, bool, error) {
	key, exists, err := s.memoryStorage.Add(ctx, userID, url)
	if err != nil {
		return 0, exists, err
	}

	if exists {
		return key, exists, err
	}

	data := fmt.Sprintf("{\"%d\":\"%s\"}\n", userID, url)
	s.fileLock.Lock()
	defer s.fileLock.Unlock()

	if _, err := s.writer.WriteString(data); err != nil {
		return key, exists, err
	}

	return key, exists, s.writer.Flush()
}

func (s *fileStorage) Get(ctx context.Context, id uint64) (string, error) {
	return s.memoryStorage.Get(ctx, id)
}

func (s *fileStorage) GetUserData(ctx context.Context, userID uint64) ([]UserData, error) {
	return s.memoryStorage.GetUserData(ctx, userID)
}

func (s *fileStorage) AddURLs(ctx context.Context, userID uint64, urls []string) ([]AddResult, error) {
	result, err := s.memoryStorage.AddURLs(ctx, userID, urls)
	if err != nil {
		return nil, err
	}

	dataToAdd := make([]string, 0)
	for i, key := range result {
		if !key.Inserted {
			continue
		}

		dataToAdd = append(dataToAdd, fmt.Sprintf("{\"%d\":\"%s\"}\n", userID, urls[i]))
	}

	insertText := strings.Join(dataToAdd, "")

	s.fileLock.Lock()
	defer s.fileLock.Unlock()

	if _, err := s.writer.WriteString(insertText); err != nil {
		return nil, err
	}

	return result, s.writer.Flush()
}

func (s *fileStorage) DeleteURLs(ctx context.Context, userID uint64, ids []uint64) error {
	return nil
}

func (s *fileStorage) Close() error {
	s.memoryStorage.Close()

	s.fileLock.Lock()
	defer s.fileLock.Unlock()

	if err := s.writer.Flush(); err != nil {
		return err
	}

	if err := s.file.Close(); err != nil {
		return err
	}
	return nil
}
