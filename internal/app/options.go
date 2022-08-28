package app

import (
	"database/sql"
	"net"

	"github.com/r4start/go-url-shortener/pkg/storage"
)

type Configurator func(s *URLShortener)

func WithDomain(domain string) Configurator {
	return func(s *URLShortener) {
		s.domain = domain
	}
}

func WithStorage(st storage.URLStorage) Configurator {
	return func(s *URLShortener) {
		s.urlStorage = st
	}
}

func WithStat(stat storage.ServiceStat) Configurator {
	return func(s *URLShortener) {
		s.stat = stat
	}
}

func WithDatabase(c *sql.DB) Configurator {
	return func(s *URLShortener) {
		s.db = c
	}
}

func WithTrustedNetwork(network *net.IPNet) Configurator {
	return func(s *URLShortener) {
		s.trustedNet = network
	}
}
