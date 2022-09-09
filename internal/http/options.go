package http

import (
	"net"
)

type ServerConfigurator func(s *Server)

func WithDomain(domain string) ServerConfigurator {
	return func(s *Server) {
		s.domain = domain
	}
}

func WithTrustedNetwork(network *net.IPNet) ServerConfigurator {
	return func(s *Server) {
		s.trustedNet = network
	}
}
