package grpc

import (
	"context"
	"log"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"go.uber.org/zap"

	"github.com/r4start/go-url-shortener/internal/app"
	pb "github.com/r4start/go-url-shortener/internal/grpc/proto"
	"github.com/r4start/go-url-shortener/pkg/storage"
)

const bufSize = 1024 * 1024

var lis *bufconn.Listener

func init() {
	logger, _ := zap.NewDevelopment()
	s, _ := app.NewURLShortener(context.Background(), logger, app.WithStorage(storage.NewInMemoryStorage()))

	_, subnet, _ := net.ParseCIDR("0.0.0.0/0")
	shortener := NewServer(s, "", subnet, logger)

	//lis = bufconn.Listen(bufSize)
	listener, _ := net.Listen("tcp", ":8099")
	grpcServer := grpc.NewServer()
	pb.RegisterUrlShortenerServer(grpcServer, shortener)
	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			log.Fatalf("Server exited with error: %v", err)
		}
	}()
}

//func bufDialer(context.Context, string) (net.Conn, error) {
//	return lis.Dial()
//}

func TestServer_Stat(t *testing.T) {
	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "localhost:8099", grpc.WithTransportCredentials(insecure.NewCredentials()))
	assert.NoError(t, err)
	defer assert.NoError(t, conn.Close())

	client := pb.NewUrlShortenerClient(conn)

	resp, err := client.Stat(ctx, &pb.StatRequest{})

	assert.NoError(t, err, "failed to retrieve stats")
	assert.Equal(t, 0, resp.Users)
	assert.Equal(t, 0, resp.Urls)
}
