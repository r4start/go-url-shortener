package app

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/r4start/go-url-shortener/pkg/storage"
	"go.uber.org/zap"

	"golang.org/x/sync/errgroup"
)

const (
	StorageOperationTimeout = time.Second

	UnlimitedWorkers     = -1
	MaxWorkersPerRequest = 5
)

type ShortenResult struct {
	Exists bool
	Key    []byte
}

type ShortenerStats struct {
	URLs  uint64
	Users uint64
}

type URLShortener struct {
	urlStorage storage.URLStorage
	stat       storage.ServiceStat
	gcm        cipher.AEAD
	privateKey []byte
	db         *sql.DB
	logger     *zap.Logger
	deleteCtx  context.Context
	deleteChan chan deleteData
	trustedNet *net.IPNet
}

func NewURLShortener(ctx context.Context, logger *zap.Logger, opts ...ShortenerConfigurator) (*URLShortener, error) {
	privateKey := make([]byte, 32)
	readBytes, err := rand.Read(privateKey)
	if err != nil || readBytes != len(privateKey) {
		return nil, err
	}
	aes256Cipher, err := aes.NewCipher(privateKey)
	if err != nil {
		return nil, err
	}

	aead, err := cipher.NewGCM(aes256Cipher)
	if err != nil {
		return nil, err
	}

	handler := &URLShortener{
		gcm:        aead,
		privateKey: privateKey,
		logger:     logger,
		deleteCtx:  ctx,
		deleteChan: make(chan deleteData),
	}

	for _, o := range opts {
		o(handler)
	}

	go handler.deleteIDs()

	return handler, nil
}

func (h *URLShortener) Shorten(ctx context.Context, userID uint64, url string) (*ShortenResult, error) {
	ctx, cancel := context.WithTimeout(ctx, StorageOperationTimeout)
	defer cancel()

	dst, exists, err := h.generateShortID(ctx, userID, url)
	if err != nil {
		return nil, err
	}

	return &ShortenResult{
		Exists: exists,
		Key:    dst,
	}, nil
}

func (h *URLShortener) OriginalURL(ctx context.Context, urlID string) (string, error) {
	key, err := decodeID(urlID)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(ctx, StorageOperationTimeout)
	defer cancel()

	return h.urlStorage.Get(ctx, key)
}

func (h *URLShortener) BatchShorten(ctx context.Context, userID uint64, urls []string) ([][]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, StorageOperationTimeout)
	defer cancel()

	return h.generateShortIDs(ctx, userID, urls)
}

func (h *URLShortener) UserURLs(ctx context.Context, userID uint64) ([]storage.UserData, error) {
	ctx, cancel := context.WithTimeout(ctx, StorageOperationTimeout)
	defer cancel()
	return h.urlStorage.GetUserData(ctx, userID)
}

func (h *URLShortener) DeleteUserURLs(_ context.Context, userID uint64, ids []string) error {
	h.deleteChan <- deleteData{
		UserID: userID,
		IDs:    ids,
	}
	return nil
}

func (h *URLShortener) Ping(ctx context.Context) error {
	if h.db == nil {
		return fmt.Errorf("no db configured")
	}

	ctx, cancel := context.WithTimeout(ctx, StorageOperationTimeout)
	defer cancel()

	return h.db.PingContext(ctx)
}

func (h *URLShortener) Stat(ctx context.Context) (*ShortenerStats, error) {
	if h.stat != nil {
		return &ShortenerStats{}, nil
	}

	ctx, cancel := context.WithTimeout(ctx, StorageOperationTimeout)
	defer cancel()

	urls, err := h.stat.TotalURLs(ctx)
	if err != nil {
		return nil, err
	}

	users, err := h.stat.TotalUsers(ctx)
	if err != nil {
		return nil, err
	}

	return &ShortenerStats{
		URLs:  urls,
		Users: users,
	}, nil
}

func (h *URLShortener) generateShortID(ctx context.Context, userID uint64, data string) ([]byte, bool, error) {
	u, err := url.Parse(data)
	if err != nil || len(u.Hostname()) == 0 {
		return nil, false, errors.New("bad input data")
	}

	key, exists, err := h.urlStorage.Add(ctx, userID, u.String())
	if err != nil {
		return nil, false, err
	}

	return encodeID(key), exists, nil
}

func (h *URLShortener) generateShortIDs(ctx context.Context, userID uint64, urls []string) ([][]byte, error) {
	for _, data := range urls {
		u, err := url.Parse(data)
		if err != nil || len(u.Hostname()) == 0 {
			return nil, errors.New("bad input data")
		}
	}

	results, err := h.urlStorage.AddURLs(ctx, userID, urls)
	if err != nil {
		return nil, err
	}

	ids := make([][]byte, 0)
	for _, key := range results {
		ids = append(ids, encodeID(key.ID))
	}

	return ids, nil
}

func (h *URLShortener) GetUserID(rawValue *string) (uint64, bool, error) {
	if rawValue == nil {
		id, err := cryptoRandUint64()
		if err != nil {
			return 0, false, err
		}
		return id, true, nil
	}

	encoder := base64.URLEncoding.WithPadding(base64.NoPadding)
	data, err := encoder.DecodeString(*rawValue)
	if err != nil {
		return 0, false, err
	}

	hasher := hmac.New(sha256.New, h.privateKey)

	if len(data) < h.gcm.NonceSize()+hasher.Size()+1 {
		return 0, false, errors.New("data size is too small")
	}

	sign := data[:hasher.Size()]
	nonce := data[hasher.Size() : hasher.Size()+h.gcm.NonceSize()]
	text := data[hasher.Size()+h.gcm.NonceSize():]

	hasher.Write(data[hasher.Size():])
	msgSign := hasher.Sum(nil)

	if !hmac.Equal(sign, msgSign) {
		id, err := cryptoRandUint64()
		if err != nil {
			return 0, false, err
		}
		return id, true, nil
	}

	var rawID []byte
	uid, err := h.gcm.Open(rawID, nonce, text, nil)
	if err != nil {
		return 0, false, err
	}

	return binary.BigEndian.Uint64(uid[:binary.MaxVarintLen64]), false, nil
}

func (h *URLShortener) GenerateUserID(userID uint64) (*string, error) {
	nonce := make([]byte, h.gcm.NonceSize())

	readBytes, err := rand.Read(nonce)
	if err != nil {
		return nil, err
	}

	if readBytes != len(nonce) {
		return nil, errors.New("not enough entropy")
	}

	text := make([]byte, binary.MaxVarintLen64)
	binary.BigEndian.PutUint64(text, userID)

	var dst []byte
	cipherText := h.gcm.Seal(dst, nonce, text, nil)
	cipherText = append(nonce, cipherText...)

	hasher := hmac.New(sha256.New, h.privateKey)
	hasher.Write(cipherText)
	sum := hasher.Sum(nil)

	cipherText = append(sum, cipherText...)
	encoder := base64.URLEncoding.WithPadding(base64.NoPadding)
	result := encoder.EncodeToString(cipherText)
	return &result, nil
}

func (h *URLShortener) deleteIDs() {
	for {
		select {
		case <-h.deleteCtx.Done():
			return
		case data := <-h.deleteChan:
			decodedIDs, err := batchDecodeIDs(h.deleteCtx, data.IDs, MaxWorkersPerRequest)
			if err != nil {
				h.logger.Error("failed to decode short ids", zap.Error(err))
				continue
			}

			if err := h.urlStorage.DeleteURLs(h.deleteCtx, data.UserID, decodedIDs); err != nil {
				h.logger.Error("failed to delete urls", zap.Error(err))
			}
		}
	}
}

func encodeID(id uint64) []byte {
	keyData := []byte(strconv.FormatUint(id, 16))
	dst := make([]byte, base64.RawURLEncoding.EncodedLen(len(keyData)))
	base64.RawURLEncoding.Encode(dst, keyData)
	return dst
}

func decodeID(data string) (uint64, error) {
	decodedKey, err := base64.RawURLEncoding.DecodeString(data)
	if err != nil {
		return 0, err
	}
	key, err := strconv.ParseUint(string(decodedKey), 16, 64)
	if err != nil {
		return 0, err
	}

	return key, nil
}

func batchDecodeIDs(ctx context.Context, strIDs []string, maxParallel int) ([]uint64, error) {
	strIDsLength := len(strIDs)
	batchSize := 1
	if maxParallel != UnlimitedWorkers {
		batchSize = strIDsLength / maxParallel
		if strIDsLength%maxParallel != 0 {
			batchSize++
		}
	}

	g, _ := errgroup.WithContext(ctx)
	ids := make([]uint64, strIDsLength)

	for i := 0; i < strIDsLength; i += batchSize {
		end := i + batchSize
		if end > strIDsLength {
			end = strIDsLength
		}
		i, idBatch := i, strIDs[i:end]
		g.Go(func() error {
			for j, id := range idBatch {
				v, err := decodeID(id)
				if err != nil {
					return err
				}

				ids[i+j] = v
			}

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return ids, nil
}

func cryptoRandUint64() (uint64, error) {
	randU64 := make([]byte, binary.MaxVarintLen64)
	if readBytes, err := rand.Read(randU64); err != nil || readBytes != binary.MaxVarintLen64 {
		if err != nil {
			return 0, err
		} else {
			return 0, errors.New("not enough entropy")
		}
	}
	return binary.BigEndian.Uint64(randU64), nil
}
