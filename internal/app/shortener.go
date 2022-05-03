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
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/r4start/go-url-shortener/internal/storage"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	UserIDCookieName = "gusid"

	StorageOperationTimeout = time.Second

	UnlimitedWorkers     = -1
	MaxWorkersPerRequest = 5
)

var ErrBadRequest = errors.New("bad request")

type apiRequestData struct {
	UserID        uint64
	IsIDGenerated bool
}

type URLShortener struct {
	*chi.Mux
	urlStorage storage.URLStorage
	domain     string
	gcm        cipher.AEAD
	privateKey []byte
	db         *sql.DB
	logger     *zap.Logger
}

func NewURLShortener(db *sql.DB, domain string, st storage.URLStorage, logger *zap.Logger) (*URLShortener, error) {
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
		Mux:        chi.NewMux(),
		urlStorage: st,
		domain:     domain,
		gcm:        aead,
		privateKey: privateKey,
		db:         db,
		logger:     logger,
	}

	handler.Use(DecompressGzip)
	handler.Use(CompressGzip)

	handler.Get("/{id}", handler.getURL)
	handler.Get("/ping", handler.ping)

	handler.Get("/api/user/urls", handler.apiUserURLs)
	handler.Delete("/api/user/urls", handler.apiDeleteUserURLs)

	handler.Post("/", handler.shorten)
	handler.Post("/api/shorten", handler.apiShortener)
	handler.Post("/api/shorten/batch", handler.apiBatchShortener)

	handler.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "", http.StatusBadRequest)
	})

	return handler, nil
}

func (h *URLShortener) shorten(w http.ResponseWriter, r *http.Request) {
	userID, generated, err := h.getUserID(r)
	if err != nil {
		h.logger.Error("failed to generate user id", zap.Error(err))
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	b, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Error("failed to read request body", zap.Error(err))
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), StorageOperationTimeout)
	defer cancel()

	dst, exists, err := h.generateShortID(ctx, userID, string(b))
	if err != nil {
		h.logger.Error("failed to generate short id", zap.Error(err))
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	if generated {
		if err := h.setUserID(w, userID); err != nil {
			h.logger.Error("failed to set user id", zap.Error(err))
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
	}

	if !exists {
		w.WriteHeader(http.StatusCreated)
	} else {
		w.WriteHeader(http.StatusConflict)
	}

	if _, err := w.Write([]byte(h.makeResultURL(r, dst))); err != nil {
		h.logger.Error("failed to write response body", zap.Error(err))
	}
}

func (h *URLShortener) getURL(w http.ResponseWriter, r *http.Request) {
	keyData := chi.URLParam(r, "id")
	key, err := decodeID(keyData)
	if err != nil {
		h.logger.Error("failed to decode short id", zap.Error(err), zap.String("id", keyData))
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), StorageOperationTimeout)
	defer cancel()

	u, err := h.urlStorage.Get(ctx, key)
	if err == storage.ErrDeleted {
		w.WriteHeader(http.StatusGone)
		return
	}

	if err != nil {
		h.logger.Error("failed to retrieve data from storage", zap.Error(err), zap.Uint64("key", key))
		http.Error(w, "", http.StatusNotFound)
		return
	}

	w.Header().Set("Location", u)
	w.WriteHeader(http.StatusTemporaryRedirect)
}

func (h *URLShortener) apiShortener(w http.ResponseWriter, r *http.Request) {
	var request map[string]string

	reqData, err := h.apiParseRequest(r, &request)
	if errors.Is(err, ErrBadRequest) {
		http.Error(w, "", http.StatusBadRequest)
		return
	} else if err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	urlToShorten, ok := request["url"]
	if !ok {
		h.logger.Error("empty url in request body")
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), StorageOperationTimeout)
	defer cancel()

	dst, exists, err := h.generateShortID(ctx, reqData.UserID, urlToShorten)
	if err != nil {
		h.logger.Error("failed to generate short id", zap.Error(err))
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	response := make(map[string]string)
	response["result"] = h.makeResultURL(r, dst)

	statusCode := http.StatusCreated
	if exists {
		statusCode = http.StatusConflict
	}
	h.apiWriteResponse(w, reqData, statusCode, response)
}

func (h *URLShortener) apiBatchShortener(w http.ResponseWriter, r *http.Request) {
	type request struct {
		CorrelationID string `json:"correlation_id"`
		OriginalURL   string `json:"original_url"`
	}

	type response struct {
		CorrelationID string `json:"correlation_id"`
		ShortURL      string `json:"short_url"`
	}

	requestData := make([]request, 0)
	reqData, err := h.apiParseRequest(r, &requestData)
	if errors.Is(err, ErrBadRequest) {
		h.logger.Error("bad request", zap.Error(err))
		http.Error(w, "", http.StatusBadRequest)
		return
	} else if err != nil {
		h.logger.Error("failed to parse request", zap.Error(err))
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	urls := make([]string, 0)
	for _, e := range requestData {
		urls = append(urls, e.OriginalURL)
	}

	ctx, cancel := context.WithTimeout(r.Context(), StorageOperationTimeout)
	defer cancel()

	encodedIds, err := h.generateShortIDs(ctx, reqData.UserID, urls)
	if err != nil {
		h.logger.Error("failed to generate short ids", zap.Error(err))
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	responseData := make([]response, 0)
	for i, dst := range encodedIds {
		responseData = append(responseData, response{
			CorrelationID: requestData[i].CorrelationID,
			ShortURL:      h.makeResultURL(r, dst),
		})
	}

	h.apiWriteResponse(w, reqData, http.StatusCreated, responseData)
}

func (h *URLShortener) apiUserURLs(w http.ResponseWriter, r *http.Request) {
	userID, generated, err := h.getUserID(r)
	if err != nil {
		h.logger.Error("failed to generate user id", zap.Error(err))
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	if generated {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), StorageOperationTimeout)
	defer cancel()
	userUrls, err := h.urlStorage.GetUserData(ctx, userID)
	if err != nil {
		h.logger.Error("failed to get user data", zap.Error(err))
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	type response struct {
		ShortURL    string `json:"short_url"`
		OriginalURL string `json:"original_url"`
	}

	result := make([]response, 0)
	for _, u := range userUrls {
		result = append(result, response{
			ShortURL:    h.makeResultURL(r, encodeID(u.ShortURLID)),
			OriginalURL: u.OriginalURL,
		})
	}

	h.apiWriteResponse(w, &apiRequestData{
		UserID: userID,
	}, http.StatusOK, result)
}

func (h *URLShortener) apiDeleteUserURLs(w http.ResponseWriter, r *http.Request) {
	requestData := make([]string, 0)
	reqData, err := h.apiParseRequest(r, &requestData)
	if errors.Is(err, ErrBadRequest) {
		h.logger.Error("bad request", zap.Error(err))
		http.Error(w, "", http.StatusBadRequest)
		return
	} else if err != nil {
		h.logger.Error("failed to parse request", zap.Error(err))
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	if reqData.IsIDGenerated {
		h.logger.Error("unknown user id")
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	ids, err := batchDecodeIDs(r.Context(), requestData, MaxWorkersPerRequest)
	if err != nil {
		h.logger.Error("failed to decode short ids", zap.Error(err))
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), StorageOperationTimeout)
	defer cancel()

	if err := h.urlStorage.DeleteURLs(ctx, reqData.UserID, ids); err != nil {
		h.logger.Error("failed to delete urls", zap.Error(err))
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

func (h *URLShortener) ping(w http.ResponseWriter, r *http.Request) {
	if h.db == nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), StorageOperationTimeout)
	defer cancel()
	if err := h.db.PingContext(ctx); err != nil {
		h.logger.Error("failed to ping database", zap.Error(err))
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
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

func (h *URLShortener) makeResultURL(r *http.Request, data []byte) string {
	if len(h.domain) != 0 {
		return fmt.Sprintf("%s/%s", h.domain, string(data))
	}
	return fmt.Sprintf("http://%s/%s", r.Host, string(data))
}

func (h *URLShortener) getUserID(r *http.Request) (uint64, bool, error) {
	userIDCookie, err := r.Cookie(UserIDCookieName)

	if err == http.ErrNoCookie {
		id, err := cryptoRandUint64()
		if err != nil {
			return 0, false, err
		}
		return id, true, nil
	} else if err != nil {
		return 0, false, err
	}

	encoder := base64.URLEncoding.WithPadding(base64.NoPadding)
	data, err := encoder.DecodeString(userIDCookie.Value)
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

func (h *URLShortener) setUserID(w http.ResponseWriter, userID uint64) error {
	nonce := make([]byte, h.gcm.NonceSize())

	readBytes, err := rand.Read(nonce)
	if err != nil {
		return err
	}

	if readBytes != len(nonce) {
		return errors.New("not enough entropy")
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
	encodedText := encoder.EncodeToString(cipherText)

	cookie := http.Cookie{
		Name:  UserIDCookieName,
		Value: encodedText,
		Path:  "/",
	}
	http.SetCookie(w, &cookie)

	return nil
}

func (h *URLShortener) apiParseRequest(r *http.Request, body interface{}) (*apiRequestData, error) {
	if contentType := r.Header.Get("Content-Type"); contentType != "application/json" {
		h.logger.Error("bad content type", zap.String("content_type", contentType))
		return nil, ErrBadRequest
	}

	userID, generated, err := h.getUserID(r)
	if err != nil {
		h.logger.Error("failed to generate user id", zap.Error(err))
		return nil, err
	}

	b, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Error("failed to read request body", zap.Error(err))
		return nil, err
	}

	if err = json.Unmarshal(b, &body); err != nil {
		h.logger.Error("failed to unmarshal request json", zap.Error(err))
		return nil, ErrBadRequest
	}
	return &apiRequestData{
		UserID:        userID,
		IsIDGenerated: generated,
	}, nil
}

func (h *URLShortener) apiWriteResponse(w http.ResponseWriter, reqData *apiRequestData, statusCode int, response interface{}) {
	dst, err := json.Marshal(response)
	if err != nil {
		h.logger.Error("failed to marshal response", zap.Error(err))
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	if reqData.IsIDGenerated {
		if err := h.setUserID(w, reqData.UserID); err != nil {
			h.logger.Error("failed to set user id", zap.Error(err))
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if _, err := w.Write(dst); err != nil {
		h.logger.Error("failed to write response body", zap.Error(err))
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
