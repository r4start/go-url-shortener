package app

import (
	"database/sql"
	"net"

	"github.com/r4start/go-url-shortener/pkg/storage"
)

type HTTPServerConfigurator func(s *HTTPServer)

func WithDomain(domain string) HTTPServerConfigurator {
	return func(s *HTTPServer) {
		s.domain = domain
	}
}

func WithTrustedNetwork(network *net.IPNet) HTTPServerConfigurator {
	return func(s *HTTPServer) {
		s.trustedNet = network
	}
}

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
