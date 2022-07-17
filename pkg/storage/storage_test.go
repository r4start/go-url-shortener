package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_generateKey(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		want    uint64
		wantErr bool
	}{
		{
			name:    "Test #1",
			url:     "",
			want:    14695981039346656037,
			wantErr: false,
		},
		{
			name:    "Test #2",
			url:     "askjdhasjkdh",
			want:    6218997846724595707,
			wantErr: false,
		},
		{
			name:    "Test #3",
			url:     "https://ya.ru",
			want:    17627783340430073139,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := generateKey(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("generateKey() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, got, tt.want)

		})
	}
}

func Test_syncMapStorage_Add(t *testing.T) {
	s := NewInMemoryStorage()
	type args struct {
		userID uint64
		url    string
	}
	tests := []struct {
		name     string
		args     args
		urlID    uint64
		hasValue bool
		wantErr  bool
	}{
		{
			name: "Test #1",
			args: args{
				userID: 1,
				url:    "vc.ru",
			},
			urlID:    0x21755717847555a5,
			hasValue: false,
			wantErr:  false,
		},
		{
			name: "Test #2",
			args: args{
				userID: 2,
				url:    "ya.ru",
			},
			urlID:    0x8db042ffceba9520,
			hasValue: true,
			wantErr:  false,
		},
	}
	_, _, err := s.Add(context.Background(), tests[1].args.userID, tests[1].args.url)
	assert.Nil(t, err)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, has, err := s.Add(context.Background(), tt.args.userID, tt.args.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("Add() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, id, tt.urlID)
			assert.Equal(t, has, tt.hasValue)
		})
	}
}

func Test_syncMapStorage_AddURLs(t *testing.T) {
	s := NewInMemoryStorage()
	type args struct {
		userID uint64
		urls   []string
	}
	tests := []struct {
		name   string
		args   args
		result []AddResult
	}{
		{
			name: "Test #1",
			args: args{
				userID: 1,
				urls: []string{
					"vc.ru",
					"ya.ru",
					"yandex.ru",
				},
			},
			result: []AddResult{
				{
					ID:       0x21755717847555a5,
					Inserted: true,
				},
				{
					ID:       0x8db042ffceba9520,
					Inserted: true,
				},
				{
					ID:       0x2247f3ac888bb083,
					Inserted: true,
				},
			},
		},
		{
			name: "Test #2",
			args: args{
				userID: 1,
				urls: []string{
					"vc.ru",
				},
			},
			result: []AddResult{
				{
					ID:       0x21755717847555a5,
					Inserted: false,
				},
			},
		},
		{
			name: "Test #3",
			args: args{
				userID: 2,
				urls: []string{
					"vc.ru",
					"ya.ru",
					"yandex.ru",
				},
			},
			result: []AddResult{
				{
					ID:       0x21755717847555a5,
					Inserted: false,
				},
				{
					ID:       0x8db042ffceba9520,
					Inserted: false,
				},
				{
					ID:       0x2247f3ac888bb083,
					Inserted: false,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := s.AddURLs(context.Background(), tt.args.userID, tt.args.urls)
			assert.Nil(t, err)
			assert.Equal(t, tt.result, result)
		})
	}
}

func Test_syncMapStorage_DeleteURLs(t *testing.T) {
	userID := uint64(1)

	type args struct {
		userID uint64
		ids    []uint64
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "Test #1",
			args: args{
				userID: userID,
				ids:    []uint64{},
			},
			wantErr: false,
		},
		{
			name: "Test #2",
			args: args{
				userID: userID,
				ids:    []uint64{0x21755717847555a5, 0x8db042ffceba9520},
			},
			wantErr: false,
		},
		{
			name: "Test #3",
			args: args{
				userID: userID + 1,
				ids:    []uint64{0x21755717847555a5, 0x2247f3ac888bb083, 0x8db042ffceba9520},
			},
			wantErr: true,
		},
		{
			name: "Test #4",
			args: args{
				userID: userID,
				ids:    []uint64{1, 2, 3},
			},
			wantErr: false,
		},
		{
			name: "Test #5",
			args: args{
				userID: userID,
				ids:    []uint64{0x2247f3ac888bb083},
			},
			wantErr: false,
		},
	}
	urls := []string{
		"vc.ru",
		"ya.ru",
		"yandex.ru",
	}
	s := NewInMemoryStorage()
	ids, err := s.AddURLs(context.Background(), userID, urls)
	assert.Nil(t, err)
	assert.NotEmpty(t, ids)

	for _, tt := range tests {
		err := s.DeleteURLs(context.Background(), tt.args.userID, tt.args.ids)
		if err != nil && !tt.wantErr {
			assert.Nil(t, err)
		} else if err == nil && tt.wantErr {
			assert.False(t, true, "[%s]Wanted error, but none was returned", tt.name)
		}
	}

	r, err := s.GetUserData(context.Background(), userID)
	assert.Nil(t, err)
	assert.Empty(t, r)
}

func Test_syncMapStorage_Get(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		id      uint64
		wantErr bool
	}{
		{
			name:    "Test #1",
			url:     "vc.ru",
			id:      0x21755717847555a5,
			wantErr: false,
		},
		{
			name:    "Test #2",
			url:     "ya.ru",
			id:      0x8db042ffceba9520,
			wantErr: false,
		},
		{
			name:    "Test #3",
			url:     "yandex.ru",
			id:      0x2247f3ac888bb083,
			wantErr: false,
		},
		{
			name:    "Test #4",
			url:     "",
			id:      0,
			wantErr: true,
		},
	}

	urls := []string{
		"vc.ru",
		"ya.ru",
		"yandex.ru",
	}
	s := NewInMemoryStorage()
	ids, err := s.AddURLs(context.Background(), 1, urls)
	assert.Nil(t, err)
	assert.NotEmpty(t, ids)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, err := s.Get(context.Background(), tt.id)
			if err != nil {
				if tt.wantErr {
					return
				}
				assert.Nil(t, err)
			} else if err == nil && tt.wantErr {
				assert.False(t, true, "[%s]Wanted error, but none was returned", tt.name)
			}

			assert.Equal(t, tt.url, url)
		})
	}
}

func Test_syncMapStorage_GetUserData(t *testing.T) {
	testUserID := uint64(1)

	tests := []struct {
		name     string
		userID   uint64
		userData []UserData
		wantErr  bool
	}{
		{
			name:   "Test #1",
			userID: testUserID,
			userData: []UserData{
				{
					ShortURLID:  0x21755717847555a5,
					OriginalURL: "vc.ru",
				},
				{
					ShortURLID:  0x8db042ffceba9520,
					OriginalURL: "ya.ru",
				},
				{
					ShortURLID:  0x2247f3ac888bb083,
					OriginalURL: "yandex.ru",
				},
			},
			wantErr: false,
		},
		{
			name:     "Test #2",
			userID:   testUserID + 1,
			userData: nil,
			wantErr:  true,
		},
		{
			name:     "Test #3",
			userID:   testUserID + 2,
			userData: []UserData{},
			wantErr:  false,
		},
	}
	urls := []string{
		"vc.ru",
		"ya.ru",
		"yandex.ru",
	}
	s := NewInMemoryStorage()
	ids, err := s.AddURLs(context.Background(), testUserID, urls)
	assert.Nil(t, err)
	assert.NotEmpty(t, ids)

	urls = []string{
		"tvc.ru",
		"tya.ru",
		"tyandex.ru",
	}
	ids, err = s.AddURLs(context.Background(), testUserID+2, urls)
	assert.Nil(t, err)
	assert.NotEmpty(t, ids)
	delIds := make([]uint64, len(ids))
	for i := 0; i < len(ids); i++ {
		delIds[i] = ids[i].ID
	}
	assert.Nil(t, s.DeleteURLs(context.Background(), testUserID+2, delIds))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := s.GetUserData(context.Background(), tt.userID)
			if err != nil {
				if tt.wantErr {
					return
				}
				assert.Nil(t, err)
			} else if err == nil && tt.wantErr {
				assert.False(t, true, "[%s]Wanted error, but none was returned", tt.name)
			}

			assert.Equal(t, tt.userData, data)
		})
	}
}

func ExampleNewInMemoryStorage() {
	ctx := context.Background()

	// Create new in-memory storage.
	s := NewInMemoryStorage()

	// It is necessary to call Close for a storage.
	defer s.Close()

	url := "https://goog.le"
	userID := uint64(1)

	// Add a URL for a user.
	urlID, hasValue, err := s.Add(ctx, userID, url)
	if err != nil {
		// Some error handling
	}

	// If we've already put a URL into storage than hasValue will be true.
	if hasValue {
		fmt.Println("Storage has already contained the URL.")
	}

	fmt.Printf("ID: %d URL: %s", urlID, url)

	// Now we can retrieve original URL by its ID.
	storageURL, err := s.Get(ctx, urlID)
	if err != nil {
		// Some error handling
	}

	fmt.Printf("Original URL: %s URL: %s", url, storageURL)

	// if you need to get all user URLs use GetUserData method
	userInfo, err := s.GetUserData(ctx, userID)
	if err != nil {
		// Some error handling
	}
	fmt.Printf("User data: %s", userInfo)

	// Delete url
	if err := s.DeleteURLs(ctx, userID, []uint64{urlID}); err != nil {
		// Some error handling
	}
}

func ExampleNewDatabaseStorage() {
	ctx := context.Background()
	connDSN := "user=postgres password=pwd host=localhost port=5432 database=postgres"

	dbConn, err := sql.Open("pgx", connDSN)
	if err != nil {
		// Some error handling
	}

	// Create new storage in a database.
	s, err := NewDatabaseStorage(ctx, dbConn)
	if err != nil {
		// Some error handling
	}
	// It is necessary to call Close for a storage.
	defer s.Close()

	url := "https://goog.le"
	userID := uint64(1)

	// Add a URL for a user.
	urlID, hasValue, err := s.Add(ctx, userID, url)
	if err != nil {
		// Some error handling
	}

	// If we've already put a URL into storage than hasValue will be true.
	if hasValue {
		fmt.Println("Storage has already contained the URL.")
	}

	fmt.Printf("ID: %d URL: %s", urlID, url)

	// Now we can retrieve original URL by its ID.
	storageURL, err := s.Get(ctx, urlID)
	if err != nil {
		// Some error handling
	}

	fmt.Printf("Original URL: %s URL: %s", url, storageURL)

	// if you need to get all user URLs use GetUserData method
	userInfo, err := s.GetUserData(ctx, userID)
	if err != nil {
		// Some error handling
	}
	fmt.Printf("User data: %s", userInfo)

	// Delete url
	if err := s.DeleteURLs(ctx, userID, []uint64{urlID}); err != nil {
		// Some error handling
	}
}

func ExampleNewFileStorage() {
	ctx := context.Background()
	filePath := "storage"

	// Create new storage in a file.
	s, err := NewFileStorage(filePath)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if err != nil {
		// Some error handling
	}
	// It is necessary to call Close for a storage.
	defer s.Close()

	url := "https://goog.le"
	userID := uint64(1)

	// Add a URL for a user.
	urlID, hasValue, err := s.Add(ctx, userID, url)
	if err != nil {
		// Some error handling
	}

	// If we've already put a URL into storage than hasValue will be true.
	if hasValue {
		fmt.Println("Storage has already contained the URL.")
	}

	fmt.Printf("ID: %d URL: %s", urlID, url)

	// Now we can retrieve original URL by its ID.
	storageURL, err := s.Get(ctx, urlID)
	if err != nil {
		// Some error handling
	}

	fmt.Printf("Original URL: %s URL: %s", url, storageURL)

	// if you need to get all user URLs use GetUserData method
	userInfo, err := s.GetUserData(ctx, userID)
	if err != nil {
		// Some error handling
	}
	fmt.Printf("User data: %s", userInfo)

	// Delete url
	if err := s.DeleteURLs(ctx, userID, []uint64{urlID}); err != nil {
		// Some error handling
	}
}
