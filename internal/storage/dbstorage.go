package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const (
	FeedsTable = "feeds"

	CreateFeedsTableScheme = `
       CREATE TABLE feeds (
			id bigserial PRIMARY KEY,
			url_hash bigint not null,
			url varchar(8192) not null UNIQUE,
			user_id bigint not null,
			added timestamptz not null DEFAULT now()
		);`

	InsertFeed = `INSERT INTO feeds (url_hash, url, user_id) VALUES (%d, '%s', %d)` +
		`ON CONFLICT ON CONSTRAINT feeds_url_key DO NOTHING;`
)

type dbRow struct {
	ID      int64
	URLHash int64
	URL     string
	UserID  int64
	Added   time.Time
}

type dbStorage struct {
	dbConn *sql.DB
}

func NewDatabaseStorage(connection *sql.DB) (URLStorage, error) {
	if err := connection.Ping(); err != nil {
		return nil, err
	}

	if err := prepareDatabase(connection); err != nil {
		return nil, err
	}

	return &dbStorage{dbConn: connection}, nil
}

func (s *dbStorage) Add(ctx context.Context, userID uint64, url string) (uint64, bool, error) {
	key, err := generateKey(&url)
	if err != nil {
		return 0, false, err
	}

	stmt := fmt.Sprintf(InsertFeed, int64(key), url, int64(userID))

	if res, err := s.dbConn.ExecContext(ctx, stmt); err != nil {
		return 0, false, err
	} else {
		if affected, err := res.RowsAffected(); err != nil {
			return 0, false, err
		} else if affected != 0 {
			return key, false, nil
		}
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

func (s *dbStorage) Get(ctx context.Context, id uint64) (string, error) {
	var url string

	stmt := fmt.Sprintf("select url from %s where url_hash = %d;", FeedsTable, int64(id))
	if err := s.dbConn.QueryRowContext(ctx, stmt).Scan(&url); err != nil {
		return "", err
	}

	return url, nil
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

func (s *dbStorage) Close() error {
	return s.dbConn.Close()
}

func prepareDatabase(conn *sql.DB) error {
	stmt := fmt.Sprintf("select count(*) from %s;", FeedsTable)
	if r, exists := conn.Query(stmt); exists != nil {
		if r, err := conn.Query(CreateFeedsTableScheme); err != nil {
			return err
		} else if err := r.Err(); err != nil {
			return err
		}
	} else if err := r.Err(); err != nil {
		return err
	}

	return nil
}
