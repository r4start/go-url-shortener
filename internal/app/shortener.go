package app

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/r4start/go-url-shortener/internal/storage"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
)

const UserIdCookieName = "gusid"

type URLShortener struct {
	*chi.Mux
	urlStorage storage.UserURLStorage
	domain     string
	gcm        cipher.AEAD
	privateKey []byte
}

func DefaultURLShortener() (*URLShortener, error) {
	return NewURLShortener("", "")
}

func NewURLShortener(domain, fileStoragePath string) (*URLShortener, error) {
	//var st storage.URLStorage
	//if len(fileStoragePath) != 0 {
	//	var err error
	//	st, err = storage.NewFileStorage(fileStoragePath)
	//	if err != nil {
	//		return nil, err
	//	}
	//} else {
	//	st = storage.NewInMemoryStorage()
	//}
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
		urlStorage: storage.NewInMemoryUserStorage(),
		domain:     domain,
		gcm:        aead,
		privateKey: privateKey,
	}

	handler.Use(DecompressGzip)
	handler.Use(CompressGzip)

	handler.Get("/{id}", handler.getURL)
	handler.Post("/", handler.shorten)
	handler.Post("/api/shorten", handler.apiShortener)

	handler.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "", http.StatusBadRequest)
	})

	return handler, nil
}

func (h *URLShortener) shorten(w http.ResponseWriter, r *http.Request) {
	userId, generated, err := h.getUserId(r)
	if err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	b, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	dst, err := h.generateShortID(userId, string(b))
	if err != nil {
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	if generated {
		if err := h.setUserId(w, userId); err != nil {
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(h.makeResultURL(r, dst)))
}

func (h *URLShortener) getURL(w http.ResponseWriter, r *http.Request) {
	keyData := chi.URLParam(r, "id")
	decodedKey, err := base64.RawURLEncoding.DecodeString(keyData)
	if err != nil {
		http.Error(w, "", http.StatusBadRequest)
		return
	}
	key, err := strconv.ParseUint(string(decodedKey), 16, 64)
	if err != nil {
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	u, err := h.urlStorage.Get(key)
	if err != nil {
		http.Error(w, "", http.StatusNotFound)
		return
	}

	w.Header().Set("Location", u)
	w.WriteHeader(http.StatusTemporaryRedirect)
}

func (h *URLShortener) apiShortener(w http.ResponseWriter, r *http.Request) {
	if contentType := r.Header.Get("Content-Type"); contentType != "application/json" {
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	userId, generated, err := h.getUserId(r)
	if err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	b, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	var request map[string]string
	if err = json.Unmarshal(b, &request); err != nil {
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	urlToShorten, ok := request["url"]
	if !ok {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	dst, err := h.generateShortID(userId, urlToShorten)
	if err != nil {
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	response := make(map[string]string)
	response["result"] = h.makeResultURL(r, dst)

	if dst, err = json.Marshal(response); err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	if generated {
		if err := h.setUserId(w, userId); err != nil {
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write(dst)
}

func (h *URLShortener) generateShortID(userId uint64, data string) ([]byte, error) {
	u, err := url.Parse(data)
	if err != nil || len(u.Hostname()) == 0 {
		return nil, errors.New("bad input data")
	}

	key, _, err := h.urlStorage.Add(userId, u.String())
	if err != nil {
		return nil, err
	}

	keyData := []byte(strconv.FormatUint(key, 16))
	dst := make([]byte, base64.RawURLEncoding.EncodedLen(len(keyData)))
	base64.RawURLEncoding.Encode(dst, keyData)

	return dst, nil
}

func (h *URLShortener) makeResultURL(r *http.Request, data []byte) string {
	if len(h.domain) != 0 {
		return fmt.Sprintf("%s/%s", h.domain, string(data))
	}
	return fmt.Sprintf("http://%s/%s", r.Host, string(data))
}

func (h *URLShortener) getUserId(r *http.Request) (uint64, bool, error) {
	userIdCookie, err := r.Cookie(UserIdCookieName)

	if err == http.ErrNoCookie {
		return rand.Uint64(), true, nil
	} else if err != nil {
		return 0, false, err
	}

	encoder := base64.URLEncoding.WithPadding(base64.NoPadding)
	data, err := encoder.DecodeString(userIdCookie.Value)
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
		return rand.Uint64(), true, nil
	}

	var rawId []byte
	uid, err := h.gcm.Open(rawId, nonce, text, nil)
	if err != nil {
		return 0, false, err
	}

	return binary.BigEndian.Uint64(uid), false, nil
}

func (h *URLShortener) setUserId(w http.ResponseWriter, userId uint64) error {
	nonce := make([]byte, h.gcm.NonceSize())

	readBytes, err := rand.Read(nonce)
	if err != nil {
		return err
	}

	if readBytes != len(nonce) {
		return errors.New("not enough entropy")
	}

	text := make([]byte, binary.MaxVarintLen64)
	binary.BigEndian.PutUint64(text, userId)

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
		Name:  UserIdCookieName,
		Value: encodedText,
		Path:  "/",
	}
	http.SetCookie(w, &cookie)

	return nil
}
