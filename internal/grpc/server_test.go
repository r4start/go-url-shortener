package grpc

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/stretchr/testify/assert"

	"go.uber.org/zap"

	"github.com/r4start/go-url-shortener/internal/app"
	pb "github.com/r4start/go-url-shortener/internal/grpc/proto"
	"github.com/r4start/go-url-shortener/pkg/storage"
)

func prepareServer(t *testing.T) (*grpc.Server, *bufconn.Listener) {
	const bufSize = 1024 * 1024

	logger, err := zap.NewDevelopment()
	assert.NoError(t, err)

	st := storage.NewInMemoryStorage()
	s, err := app.NewURLShortener(context.Background(), logger, app.WithStorage(st), app.WithStat(st))
	assert.NoError(t, err)

	shortener := NewServer(s, "", logger, func(context.Context, *pb.StatRequest) bool {
		return true
	})

	lis := bufconn.Listen(bufSize)
	grpcServer := grpc.NewServer()
	pb.RegisterUrlShortenerServer(grpcServer, shortener)
	go func(t *testing.T) {
		err := grpcServer.Serve(lis)
		assert.NoError(t, err)
	}(t)

	return grpcServer, lis
}

func makeDialer(l *bufconn.Listener) func(context.Context, string) (net.Conn, error) {
	return func(context.Context, string) (net.Conn, error) {
		return l.Dial()
	}
}

func prepareTestEnv(t *testing.T) (*grpc.Server, *grpc.ClientConn) {
	grpcServer, listener := prepareServer(t)

	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "", grpc.WithContextDialer(makeDialer(listener)),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	assert.NoError(t, err)
	return grpcServer, conn
}

func TestServer_Shorten(t *testing.T) {
	type expected struct {
		expectedURL string
	}
	tests := []struct {
		name    string
		request *pb.ShortenerRequest
		expected
	}{
		{
			name: "Grpc Shorten #1",
			request: &pb.ShortenerRequest{
				Url: "https://ya.ru",
			},
			expected: expected{
				expectedURL: "ZjRhMjc3OGQ1N2UyMWQzMw",
			},
		},
		{
			name: "Grpc Shorten #2",
			request: &pb.ShortenerRequest{
				Url: "https://vc.ru",
			},
			expected: expected{
				expectedURL: "M2U4OWJmNzU4ZWNkZTZlYQ",
			},
		},
		{
			name: "Grpc Shorten #3",
			request: &pb.ShortenerRequest{
				Url: "http://a.a",
			},
			expected: expected{
				expectedURL: "ZGNmOWU5NzRmZWZmZTRm",
			},
		},
	}

	grpcServer, conn := prepareTestEnv(t)
	defer grpcServer.Stop()
	defer conn.Close()
	client := pb.NewUrlShortenerClient(conn)

	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := client.Shorten(ctx, tt.request)
			assert.NoError(t, err)
			assert.NotZero(t, len(resp.Url))
			assert.Equal(t, tt.expectedURL, resp.Url)
		})
	}

	_, err := client.Shorten(ctx, tests[0].request)
	assert.Equal(t, codes.AlreadyExists, status.Convert(err).Code())
}

func TestServer_GetUrl(t *testing.T) {
	tests := []struct {
		name    string
		request *pb.ShortenerRequest
	}{
		{
			name: "Grpc get url check #1",
			request: &pb.ShortenerRequest{
				Url: "https://ya.ru",
			},
		},
		{
			name: "Grpc get url #2",
			request: &pb.ShortenerRequest{
				Url: "https://vc.ru",
			},
		},
		{
			name: "Grpc get url #3",
			request: &pb.ShortenerRequest{
				Url: "http://a.a",
			},
		},
	}

	grpcServer, conn := prepareTestEnv(t)
	defer grpcServer.Stop()
	defer conn.Close()
	client := pb.NewUrlShortenerClient(conn)

	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := client.Shorten(ctx, tt.request)
			assert.NoError(t, err)
			assert.NotZero(t, len(resp.Url))

			ur, err := client.GetUrl(ctx, &pb.ShortenerRequest{Url: resp.Url})
			assert.NoError(t, err)
			assert.Equal(t, tt.request.Url, ur.Url)
		})
	}
}

func TestServer_BatchShorten(t *testing.T) {
	tests := []struct {
		name     string
		request  *pb.BatchRequest
		expected []pb.BatchResponse_Result
	}{
		{
			name: "Batch shorten check #1",
			request: &pb.BatchRequest{
				Urls: []*pb.BatchRequest_UrlData{
					&pb.BatchRequest_UrlData{
						CorrelationId: 0,
						Url:           "http://ya.ru",
					},
					&pb.BatchRequest_UrlData{
						CorrelationId: 1,
						Url:           "http://vc.ru",
					},
					&pb.BatchRequest_UrlData{
						CorrelationId: 2,
						Url:           "http://habr.ru",
					},
					&pb.BatchRequest_UrlData{
						CorrelationId: 9,
						Url:           "http://lenta.ru",
					},
					&pb.BatchRequest_UrlData{
						CorrelationId: 7,
						Url:           "http://ok.ru",
					},
				},
			},
			expected: []pb.BatchResponse_Result{
				pb.BatchResponse_Result{
					CorrelationId: 0,
					Key:           "ZDIyNDk4MzQzMGZmMDQ1ZQ",
				},
				pb.BatchResponse_Result{
					CorrelationId: 1,
					Key:           "NWI4NTMwNmZjNWJmMjMzYg",
				},
				pb.BatchResponse_Result{
					CorrelationId: 2,
					Key:           "NGViNTExNTZlMzI2NmNiMw",
				},
				pb.BatchResponse_Result{
					CorrelationId: 9,
					Key:           "ZTdjMTdjZDVlMTY3YjQ1YQ",
				},
				pb.BatchResponse_Result{
					CorrelationId: 7,
					Key:           "MWE5MGMyYWI3OTVmNDRjZQ",
				},
			},
		},
	}

	grpcServer, conn := prepareTestEnv(t)
	defer grpcServer.Stop()
	defer conn.Close()
	client := pb.NewUrlShortenerClient(conn)

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := client.BatchShorten(ctx, tt.request)
			assert.NoError(t, err)
			assert.NotZero(t, len(resp.Keys))

			for i, e := range resp.Keys {
				assert.Equal(t, tt.expected[i].CorrelationId, e.CorrelationId)
				assert.Equal(t, tt.expected[i].Key, e.Key)

				ur, err := client.GetUrl(ctx, &pb.ShortenerRequest{Url: e.Key})
				assert.NoError(t, err)
				assert.Equal(t, tt.request.Urls[i].Url, ur.Url)
			}
		})
	}
}

func TestServer_ListUserUrls(t *testing.T) {
	tests := []struct {
		name     string
		request  *pb.BatchRequest
		expected map[string]string
	}{
		{
			name: "Shortener check #1",
			request: &pb.BatchRequest{
				Urls: []*pb.BatchRequest_UrlData{
					&pb.BatchRequest_UrlData{
						CorrelationId: 0,
						Url:           "http://ya.ru",
					},
					&pb.BatchRequest_UrlData{
						CorrelationId: 1,
						Url:           "http://vc.ru",
					},
					&pb.BatchRequest_UrlData{
						CorrelationId: 2,
						Url:           "http://habr.ru",
					},
					&pb.BatchRequest_UrlData{
						CorrelationId: 9,
						Url:           "http://lenta.ru",
					},
					&pb.BatchRequest_UrlData{
						CorrelationId: 7,
						Url:           "http://ok.ru",
					},
				},
			},
			expected: map[string]string{
				"http://ya.ru":    "ZDIyNDk4MzQzMGZmMDQ1ZQ",
				"http://vc.ru":    "NWI4NTMwNmZjNWJmMjMzYg",
				"http://habr.ru":  "NGViNTExNTZlMzI2NmNiMw",
				"http://lenta.ru": "ZTdjMTdjZDVlMTY3YjQ1YQ",
				"http://ok.ru":    "MWE5MGMyYWI3OTVmNDRjZQ",
			},
		},
	}

	grpcServer, conn := prepareTestEnv(t)
	defer grpcServer.Stop()
	defer conn.Close()
	client := pb.NewUrlShortenerClient(conn)

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := client.BatchShorten(ctx, tt.request)
			assert.NoError(t, err)
			assert.NotZero(t, len(resp.Keys))

			urls, err := client.ListUserUrls(ctx, &pb.ListUserUrlsRequest{UserId: *resp.UserId})
			assert.NoError(t, err)
			assert.Equal(t, len(tt.request.Urls), len(urls.Urls))
			for _, u := range urls.Urls {
				value, ok := tt.expected[u.OriginalUrl]
				assert.True(t, ok)
				assert.Equal(t, value, u.ShortUrl)
			}
		})
	}
}

func TestServer_DeleteUserUrls(t *testing.T) {
	grpcServer, conn := prepareTestEnv(t)
	defer grpcServer.Stop()
	defer conn.Close()
	client := pb.NewUrlShortenerClient(conn)

	ctx := context.Background()

	request := &pb.BatchRequest{
		Urls: []*pb.BatchRequest_UrlData{
			&pb.BatchRequest_UrlData{
				CorrelationId: 0,
				Url:           "http://ya.ru",
			},
			&pb.BatchRequest_UrlData{
				CorrelationId: 1,
				Url:           "http://vc.ru",
			},
			&pb.BatchRequest_UrlData{
				CorrelationId: 2,
				Url:           "http://habr.ru",
			},
			&pb.BatchRequest_UrlData{
				CorrelationId: 9,
				Url:           "http://lenta.ru",
			},
			&pb.BatchRequest_UrlData{
				CorrelationId: 7,
				Url:           "http://ok.ru",
			},
		},
	}

	resp, err := client.BatchShorten(ctx, request)
	assert.NoError(t, err)
	assert.NotZero(t, len(resp.Keys))

	deleteRequest := &pb.DeleteUserUrlsRequest{
		UserId: *resp.UserId,
		Urls: []string{
			resp.Keys[0].Key,
			resp.Keys[1].Key,
			resp.Keys[3].Key,
		},
	}

	_, err = client.DeleteUserUrls(ctx, deleteRequest)
	assert.NoError(t, err)

	stat, err := client.Stat(ctx, &pb.StatRequest{})
	assert.NoError(t, err)
	assert.Equal(t, uint64(1), stat.Users)
	assert.Equal(t, uint64(len(request.Urls)-len(deleteRequest.Urls)), stat.Urls)
}

func TestServer_Stat(t *testing.T) {
	grpcServer, conn := prepareTestEnv(t)
	defer grpcServer.Stop()
	defer conn.Close()
	client := pb.NewUrlShortenerClient(conn)

	ctx := context.Background()

	request := &pb.BatchRequest{
		Urls: []*pb.BatchRequest_UrlData{
			&pb.BatchRequest_UrlData{
				CorrelationId: 0,
				Url:           "http://ya.ru",
			},
			&pb.BatchRequest_UrlData{
				CorrelationId: 1,
				Url:           "http://vc.ru",
			},
			&pb.BatchRequest_UrlData{
				CorrelationId: 2,
				Url:           "http://habr.ru",
			},
			&pb.BatchRequest_UrlData{
				CorrelationId: 9,
				Url:           "http://lenta.ru",
			},
			&pb.BatchRequest_UrlData{
				CorrelationId: 7,
				Url:           "http://ok.ru",
			},
		},
	}

	resp, err := client.BatchShorten(ctx, request)
	assert.NoError(t, err)
	assert.NotZero(t, len(resp.Keys))

	stat, err := client.Stat(ctx, &pb.StatRequest{})
	assert.NoError(t, err)
	assert.Equal(t, uint64(1), stat.Users)
	assert.Equal(t, uint64(len(request.Urls)), stat.Urls)

	firstLen := stat.Urls

	request = &pb.BatchRequest{
		Urls: []*pb.BatchRequest_UrlData{
			&pb.BatchRequest_UrlData{
				CorrelationId: 0,
				Url:           "http://yandex.ru",
			},
			&pb.BatchRequest_UrlData{
				CorrelationId: 1,
				Url:           "http://dzen.ru",
			},
		},
	}

	resp, err = client.BatchShorten(ctx, request)
	assert.NoError(t, err)
	assert.NotZero(t, len(resp.Keys))

	stat, err = client.Stat(ctx, &pb.StatRequest{})
	assert.NoError(t, err)
	assert.Equal(t, uint64(2), stat.Users)
	assert.Equal(t, uint64(len(request.Urls))+firstLen, stat.Urls)
}

func TestServer_Stat_Denied(t *testing.T) {
	const bufSize = 1024 * 1024

	ctx := context.Background()

	logger, err := zap.NewDevelopment()
	assert.NoError(t, err)

	st := storage.NewInMemoryStorage()
	s, err := app.NewURLShortener(context.Background(), logger, app.WithStorage(st), app.WithStat(st))
	assert.NoError(t, err)

	_, trustedNet, err := net.ParseCIDR("10.0.0.1/8")
	assert.NoError(t, err)
	shortener := NewServer(s, "", logger, DefaultStatAuth(trustedNet))

	lis := bufconn.Listen(bufSize)
	grpcServer := grpc.NewServer()
	pb.RegisterUrlShortenerServer(grpcServer, shortener)
	go func(t *testing.T) {
		err := grpcServer.Serve(lis)
		assert.NoError(t, err)
	}(t)
	defer grpcServer.Stop()

	conn, err := grpc.DialContext(ctx, "", grpc.WithContextDialer(makeDialer(lis)),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	assert.NoError(t, err)
	defer conn.Close()
	client := pb.NewUrlShortenerClient(conn)

	request := &pb.BatchRequest{
		Urls: []*pb.BatchRequest_UrlData{
			&pb.BatchRequest_UrlData{
				CorrelationId: 0,
				Url:           "http://ya.ru",
			},
			&pb.BatchRequest_UrlData{
				CorrelationId: 1,
				Url:           "http://vc.ru",
			},
			&pb.BatchRequest_UrlData{
				CorrelationId: 2,
				Url:           "http://habr.ru",
			},
			&pb.BatchRequest_UrlData{
				CorrelationId: 9,
				Url:           "http://lenta.ru",
			},
			&pb.BatchRequest_UrlData{
				CorrelationId: 7,
				Url:           "http://ok.ru",
			},
		},
	}

	resp, err := client.BatchShorten(ctx, request)
	assert.NoError(t, err)
	assert.NotZero(t, len(resp.Keys))

	_, err = client.Stat(ctx, &pb.StatRequest{})
	assert.Equal(t, codes.PermissionDenied, status.Convert(err).Code())
}
