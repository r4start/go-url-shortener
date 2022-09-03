package grpc

import (
	"context"
	"net"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"

	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"github.com/r4start/go-url-shortener/internal/app"
	pb "github.com/r4start/go-url-shortener/internal/grpc/proto"
)

type Server struct {
	pb.UnimplementedUrlShortenerServer

	shortener  *app.URLShortener
	domain     string
	logger     *zap.Logger
	trustedNet *net.IPNet
}

func NewServer(shortener *app.URLShortener, domain string, trustedNetwork *net.IPNet, logger *zap.Logger) *Server {
	return &Server{
		shortener:  shortener,
		domain:     domain,
		logger:     logger,
		trustedNet: trustedNetwork,
	}
}

func (s *Server) Serve(address string) error {
	return nil
}

func (s *Server) Shorten(context.Context, *pb.ShortenerRequest) (*pb.ShortenerResponse, error) {
	return nil, nil
}

func (s *Server) BatchShorten(context.Context, *pb.BatchRequest) (*pb.BatchResponse, error) {
	return nil, nil
}

func (s *Server) GetURL(context.Context, *pb.ShortenerRequest) (*pb.ShortenerResponse, error) {
	return nil, nil
}

func (s *Server) ListUserUrls(context.Context, *pb.ListUserUrlsRequest) (*pb.ListUserUrlsResponse, error) {
	return nil, nil
}

func (s *Server) DeleteUserUrls(context.Context, *pb.DeleteUserUrlsRequest) (*pb.DeleteUserUrlsResponse, error) {
	return nil, nil
}

func (s *Server) Stat(ctx context.Context, req *pb.StatRequest) (*pb.StatResponse, error) {
	p, ok := peer.FromContext(ctx)
	if !ok || s.trustedNet == nil {
		return nil, status.Error(codes.PermissionDenied, "unauthorized client")
	}

	host, _, err := net.SplitHostPort(p.Addr.String())
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, "unauthorized client")
	}

	ip := net.ParseIP(host)
	if ip == nil || !s.trustedNet.Contains(ip) {
		return nil, status.Error(codes.PermissionDenied, "unauthorized client")
	}

	stat, err := s.shortener.Stat(ctx)
	if err != nil {
		return nil, err
	}

	return &pb.StatResponse{
		Urls:  stat.URLs,
		Users: stat.Users,
	}, nil
}
