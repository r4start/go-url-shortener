package app

import (
	"database/sql"

	"github.com/r4start/go-url-shortener/pkg/storage"
)

type ShortenerConfigurator func(s *URLShortener)

func WithStorage(st storage.URLStorage) ShortenerConfigurator {
	return func(s *URLShortener) {
		s.urlStorage = st
	}
}

func WithStat(stat storage.ServiceStat) ShortenerConfigurator {
	return func(s *URLShortener) {
		s.stat = stat
	}
}

func WithDatabase(c *sql.DB) ShortenerConfigurator {
	return func(s *URLShortener) {
		s.db = c
	}
}
