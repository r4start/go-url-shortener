package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const (
	FeedsTable = "feeds"

	StateActive   = "active"
	StateDisabled = "disabled"

	CreateStateEnum = `create type state as enum ('active', 'disabled');`

	CreateFeedsTableScheme = `
       CREATE TABLE feeds (
			id bigserial PRIMARY KEY,
			url_hash bigint not null ,
			url varchar(8192) not null UNIQUE,
			user_id bigint not null,
			added timestamptz not null DEFAULT now(),
			flags state not null DEFAULT 'active'
		);`

	CreateURLHashIndex = `create index url_hash_idx on feeds(url_hash);`

	CreateUserIDIndex = `create index user_id_idx on feeds(user_id);`

	InsertFeed = `INSERT INTO feeds (url_hash, url, user_id) VALUES (%d, '%s', %d)` +
		`ON CONFLICT ON CONSTRAINT feeds_url_key DO NOTHING;`

	DeleteFeed = `update feeds set flags = 'disabled' where url_hash = $1 and user_id = $2;`

	DatabaseFlushTimeout    = 10 * time.Second
	DatabaseDeleteQueueSize = 1000
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

func NewDatabaseStorage(ctx context.Context, connection *sql.DB) (URLStorage, error) {
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
	key, err := generateKey(&url)
	if err != nil {
		return 0, false, err
	}

	stmt := fmt.Sprintf(InsertFeed, int64(key), url, int64(userID))
	res, err := s.dbConn.ExecContext(ctx, stmt)
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
		key, err := generateKey(&url)
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

	stmt, err := tx.PrepareContext(ctx, "INSERT INTO feeds (url_hash, url, user_id) VALUES ($1, $2, $3)")
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

	stmt := fmt.Sprintf("select url, flags from %s where url_hash = %d;", FeedsTable, int64(id))
	if err := s.dbConn.QueryRowContext(ctx, stmt).Scan(&url, &state); err != nil {
		return "", err
	}

	if state == StateDisabled {
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
	stmt := fmt.Sprintf("select url_hash, url from %s where user_id = %d;", FeedsTable, int64(userID))
	rows, err := s.dbConn.QueryContext(ctx, stmt)
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

func (s *dbStorage) deleteURLs() {
	deleteQueue := make(map[uint64][]uint64)
	ticker := time.NewTicker(DatabaseFlushTimeout)
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

			if queueSize > DatabaseDeleteQueueSize {
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

	stmt, err := tx.PrepareContext(ctx, DeleteFeed)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, key := range ids {
		if _, err := stmt.ExecContext(ctx, int64(key), int64(userID)); err != nil {
			return err
		}

	}

	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}

func prepareDatabase(conn *sql.DB) error {
	stmt := fmt.Sprintf("select count(*) from %s;", FeedsTable)

	r, exists := conn.Query(stmt)
	if exists == nil {
		return r.Err()
	}

	r, err := conn.Query(CreateStateEnum)
	if err != nil {
		return err
	}
	if err := r.Err(); err != nil {
		return err
	}

	r, err = conn.Query(CreateFeedsTableScheme)
	if err != nil {
		return err
	}
	if err := r.Err(); err != nil {
		return err
	}

	r, err = conn.Query(CreateURLHashIndex)
	if err != nil {
		return err
	}
	if err := r.Err(); err != nil {
		return err
	}

	r, err = conn.Query(CreateUserIDIndex)
	if err != nil {
		return err
	}

	return r.Err()
}
