package http

import (
	"net"
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
