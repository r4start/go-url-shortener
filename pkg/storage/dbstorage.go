package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	stateActive   = "active"
	stateDisabled = "disabled"

	createStateEnum = `create type state as enum ('active', 'disabled');`

	createFeedsTableScheme = `
       CREATE TABLE feeds (
			id bigserial PRIMARY KEY,
			url_hash bigint not null,
			url varchar(8192) not null UNIQUE,
			user_id bigint not null,
			added timestamptz not null DEFAULT now(),
			flags state not null DEFAULT 'active'
		);`

	createURLHashIndex = `create index url_hash_idx on feeds(url_hash);`

	createUserIDIndex = `create index user_id_idx on feeds(user_id);`

	insertFeed = `INSERT INTO feeds (url_hash, url, user_id) VALUES ($1, $2, $3)` +
		`ON CONFLICT ON CONSTRAINT feeds_url_key DO NOTHING;`

	deleteFeed = `update feeds set flags = 'disabled' where user_id = %d and url_hash in (%s);`

	getFeed = `select url, flags from feeds where url_hash = $1;`

	getUserData = `select url_hash, url from feeds where user_id = $1 and flags = 'active';`

	checkFeedsTable = `select count(*) from feeds;`

	databaseFlushTimeout    = 10 * time.Second
	databaseDeleteQueueSize = 1000
)

var (
	_ URLStorage  = (*dbStorage)(nil)
	_ ServiceStat = (*dbStorage)(nil)
)

type dbRow struct {
	ID      int64
	URLHash int64
	URL     string
	UserID  int64
	Added   time.Time
}

type deleteEntry struct {
	UserID uint64
	IDs    []uint64
}

type dbStorage struct {
	dbConn     *sql.DB
	ctx        context.Context
	ctxCancel  context.CancelFunc
	deleteChan chan deleteEntry
}

// NewDatabaseStorage creates URLStorage implementation that defines methods over PostgreSQL database.
func NewDatabaseStorage(ctx context.Context, connection *sql.DB) (*dbStorage, error) {
	if err := connection.Ping(); err != nil {
		return nil, err
	}

	if err := prepareDatabase(connection); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	storage := &dbStorage{
		dbConn:     connection,
		ctx:        ctx,
		ctxCancel:  cancel,
		deleteChan: make(chan deleteEntry),
	}

	go storage.deleteURLs()

	return storage, nil
}

func (s *dbStorage) Add(ctx context.Context, userID uint64, url string) (uint64, bool, error) {
	key, err := generateKey(url)
	if err != nil {
		return 0, false, err
	}

	res, err := s.dbConn.ExecContext(ctx, insertFeed, int64(key), url, int64(userID))
	if err != nil {
		return 0, false, err
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return 0, false, err
	}

	if affected != 0 {
		return key, false, nil
	}

	return key, true, nil
}

func (s *dbStorage) AddURLs(ctx context.Context, userID uint64, urls []string) ([]AddResult, error) {
	keys := make([]uint64, 0)
	for _, url := range urls {
		key, err := generateKey(url)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}

	tx, err := s.dbConn.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, insertFeed)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	result := make([]AddResult, 0)
	for i, key := range keys {
		if stmtResult, err := stmt.ExecContext(ctx, int64(key), urls[i], int64(userID)); err != nil {
			return nil, err
		} else if count, err := stmtResult.RowsAffected(); err != nil {
			return nil, err
		} else {
			result = append(result, AddResult{
				ID:       key,
				Inserted: count > 0,
			})
		}

	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return result, nil
}

func (s *dbStorage) DeleteURLs(_ context.Context, userID uint64, ids []uint64) error {
	entry := deleteEntry{
		UserID: userID,
		IDs:    make([]uint64, len(ids)),
	}
	copy(entry.IDs, ids)
	s.deleteChan <- entry

	return nil
}

func (s *dbStorage) Get(ctx context.Context, id uint64) (string, error) {
	var url string
	var state string

	if err := s.dbConn.QueryRowContext(ctx, getFeed, int64(id)).Scan(&url, &state); err != nil {
		return "", err
	}

	if state == stateDisabled {
		return "", ErrDeleted
	}

	return url, nil
}

func (s *dbStorage) Close() error {
	s.ctxCancel()
	close(s.deleteChan)

	return s.dbConn.Close()
}

func (s *dbStorage) GetUserData(ctx context.Context, userID uint64) ([]UserData, error) {
	data := make([]UserData, 0)
	rows, err := s.dbConn.QueryContext(ctx, getUserData, int64(userID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var r dbRow
		if err := rows.Scan(&r.URLHash, &r.URL); err != nil {
			return nil, err
		}

		data = append(data, UserData{
			ShortURLID:  uint64(r.URLHash),
			OriginalURL: r.URL,
		})
	}

	err = rows.Err()
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (s *dbStorage) TotalUsers(ctx context.Context) (uint64, error) {
	return 0, nil
}

func (s *dbStorage) TotalURLs(ctx context.Context) (uint64, error) {
	return 0, nil
}

func (s *dbStorage) deleteURLs() {
	deleteQueue := make(map[uint64][]uint64)
	ticker := time.NewTicker(databaseFlushTimeout)
	queueSize := 0

	flush := func() {
		for userID, ids := range deleteQueue {
			t, cancel := context.WithTimeout(s.ctx, time.Second)
			s.deleteUserURLs(t, userID, ids)
			cancel()
		}
		deleteQueue = make(map[uint64][]uint64)
		queueSize = 0
	}

	for {
		select {
		case v := <-s.deleteChan:
			queueSize += len(v.IDs)
			if _, ok := deleteQueue[v.UserID]; !ok {
				deleteQueue[v.UserID] = make([]uint64, 0)
			}
			deleteQueue[v.UserID] = append(deleteQueue[v.UserID], v.IDs...)

			if queueSize > databaseDeleteQueueSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-s.ctx.Done():
			flush()
			return
		}
	}
}

func (s *dbStorage) deleteUserURLs(ctx context.Context, userID uint64, ids []uint64) error {
	tx, err := s.dbConn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	deleteIDs := make([]string, len(ids))
	for i, e := range ids {
		deleteIDs[i] = strconv.FormatInt(int64(e), 10)
	}
	stmt := fmt.Sprintf(deleteFeed, int64(userID), strings.Join(deleteIDs, ","))

	if _, err := tx.ExecContext(ctx, stmt); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}

func prepareDatabase(conn *sql.DB) error {
	r, exists := conn.Query(checkFeedsTable)
	if exists == nil {
		return r.Err()
	}

	r, err := conn.Query(createStateEnum)
	if err != nil {
		return err
	}
	if err := r.Err(); err != nil {
		return err
	}

	r, err = conn.Query(createFeedsTableScheme)
	if err != nil {
		return err
	}
	if err := r.Err(); err != nil {
		return err
	}

	r, err = conn.Query(createURLHashIndex)
	if err != nil {
		return err
	}
	if err := r.Err(); err != nil {
		return err
	}

	r, err = conn.Query(createUserIDIndex)
	if err != nil {
		return err
	}

	return r.Err()
}
