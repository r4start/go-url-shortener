package grpc

import (
	"context"
	"errors"
	"net"
	"net/http"

	"github.com/r4start/go-url-shortener/pkg/storage"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"

	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"github.com/r4start/go-url-shortener/internal/app"
	pb "github.com/r4start/go-url-shortener/internal/grpc/proto"
)

type Server struct {
	pb.UnimplementedUrlShortenerServer

	shortener     *app.URLShortener
	domain        string
	logger        *zap.Logger
	statisticAuth StatAuthorizer
}

func NewServer(shortener *app.URLShortener, domain string, logger *zap.Logger, statAuth StatAuthorizer) *Server {
	return &Server{
		shortener:     shortener,
		domain:        domain,
		logger:        logger,
		statisticAuth: statAuth,
	}
}

func (s *Server) Shorten(ctx context.Context, req *pb.ShortenerRequest) (*pb.ShortenerResponse, error) {
	userID, _, err := s.shortener.GetUserID(req.UserId)
	if err != nil {
		s.logger.Error("failed to get user id", zap.Error(err))
		return nil, status.Error(codes.Unknown, "")
	}

	r, err := s.shortener.Shorten(ctx, userID, req.Url)
	if err != nil {
		s.logger.Error("failed to generate short id", zap.Error(err))
		return nil, status.Error(codes.InvalidArgument, "")
	}

	if r.Exists {
		return nil, status.Error(codes.AlreadyExists, "")
	}

	res := &pb.ShortenerResponse{Url: string(r.Key)}

	res.UserId, err = s.shortener.GenerateUserID(userID)
	if err != nil {
		s.logger.Error("failed to generate user id", zap.Error(err))
		return nil, status.Error(codes.Unknown, "")
	}

	return res, nil
}

func (s *Server) BatchShorten(ctx context.Context, req *pb.BatchRequest) (*pb.BatchResponse, error) {
	userID, _, err := s.shortener.GetUserID(req.UserId)
	if err != nil {
		s.logger.Error("failed to get user id", zap.Error(err))
		return nil, status.Error(codes.Unknown, "")
	}

	urls := make([]string, len(req.Urls))
	for i, e := range req.Urls {
		urls[i] = e.Url
	}

	encodedIds, err := s.shortener.BatchShorten(ctx, userID, urls)
	if err != nil {
		s.logger.Error("failed to generate short ids", zap.Error(err))
		return nil, status.Error(codes.InvalidArgument, "")
	}

	res := &pb.BatchResponse{}
	res.UserId, err = s.shortener.GenerateUserID(userID)
	if err != nil {
		s.logger.Error("failed to generate user id", zap.Error(err))
		return nil, status.Error(codes.Unknown, "")
	}

	res.Keys = make([]*pb.BatchResponse_Result, len(encodedIds))
	for i, e := range encodedIds {
		res.Keys[i] = &pb.BatchResponse_Result{
			CorrelationId: req.Urls[i].CorrelationId,
			Key:           string(e),
		}
	}

	return res, nil
}

func (s *Server) GetURL(ctx context.Context, req *pb.ShortenerRequest) (*pb.ShortenerResponse, error) {
	u, err := s.shortener.OriginalURL(ctx, req.Url)
	if errors.Is(err, storage.ErrDeleted) {
		return nil, status.Error(http.StatusGone, "")
	} else if err != nil {
		s.logger.Error("failed to get original url", zap.Error(err))
		return nil, status.Error(codes.NotFound, "")
	}
	return &pb.ShortenerResponse{Url: u}, nil
}

func (s *Server) ListUserUrls(ctx context.Context, req *pb.ListUserUrlsRequest) (*pb.ListUserUrlsResponse, error) {
	userID, generated, err := s.shortener.GetUserID(&req.UserId)
	if err != nil {
		s.logger.Error("failed to get user id", zap.Error(err))
		return nil, status.Error(codes.Unknown, "")
	}

	if generated {
		return nil, status.Error(codes.Unauthenticated, "")
	}

	userUrls, err := s.shortener.UserURLs(ctx, userID)
	if err != nil {
		s.logger.Error("failed to get user data", zap.Error(err))
		return nil, status.Error(codes.Unknown, "")
	}

	result := &pb.ListUserUrlsResponse{
		Urls: make([]*pb.ListUserUrlsResponse_Result, 0, len(userUrls)),
	}

	for _, e := range userUrls {
		result.Urls = append(result.Urls, &pb.ListUserUrlsResponse_Result{
			ShortUrl:    string(app.EncodeID(e.ShortURLID)),
			OriginalUrl: e.OriginalURL,
		})
	}

	return result, nil
}

func (s *Server) DeleteUserUrls(ctx context.Context, req *pb.DeleteUserUrlsRequest) (*pb.DeleteUserUrlsResponse, error) {
	userID, generated, err := s.shortener.GetUserID(&req.UserId)
	if err != nil {
		s.logger.Error("failed to get user id", zap.Error(err))
		return nil, status.Error(codes.Unknown, "")
	}

	if generated {
		return nil, status.Error(codes.Unauthenticated, "")
	}

	if err := s.shortener.DeleteUserURLs(ctx, userID, req.Urls); err != nil {
		s.logger.Error("failed to delete user urls", zap.Error(err))
		return nil, status.Error(codes.Unknown, "")
	}

	return &pb.DeleteUserUrlsResponse{}, nil
}

func (s *Server) Stat(ctx context.Context, req *pb.StatRequest) (*pb.StatResponse, error) {
	if !s.statisticAuth(ctx, req) {
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

type StatAuthorizer func(context.Context, *pb.StatRequest) bool

func DefaultStatAuth(trustedNetwork *net.IPNet) StatAuthorizer {
	return func(ctx context.Context, request *pb.StatRequest) bool {
		p, ok := peer.FromContext(ctx)
		if !ok || trustedNetwork == nil {
			return false
		}

		host, _, err := net.SplitHostPort(p.Addr.String())
		if err != nil {
			return false
		}

		ip := net.ParseIP(host)
		if ip == nil || !trustedNetwork.Contains(ip) {
			return false
		}
		return true
	}
}
