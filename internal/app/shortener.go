package app

import (
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/r4start/go-url-shortener/internal/storage"
)

type URLShortener struct {
	*chi.Mux
	urlStorage storage.URLStorage
	domain     string
}

func DefaultURLShortener() (*URLShortener, error) {
	return NewURLShortener("", "")
}

func NewURLShortener(domain, fileStoragePath string) (*URLShortener, error) {
	var st storage.URLStorage
	if len(fileStoragePath) != 0 {
		var err error
		st, err = storage.NewFileStorage(fileStoragePath)
		if err != nil {
			return nil, err
		}
	} else {
		st = storage.NewInMemoryStorage()
	}
	handler := &URLShortener{Mux: chi.NewMux(), urlStorage: st, domain: domain}

	handler.Use(DecompressGzip)
	handler.Use(middleware.Compress(gzip.BestCompression))

	handler.Get("/{id}", handler.getURL)
	handler.Post("/", handler.shorten)
	handler.Post("/api/shorten", handler.apiShortener)

	handler.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "", http.StatusBadRequest)
	})

	return handler, nil
}

func (h *URLShortener) shorten(w http.ResponseWriter, r *http.Request) {
	b, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	dst, err := h.generateShortID(string(b))
	if err != nil {
		http.Error(w, "", http.StatusBadRequest)
		return
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

	dst, err := h.generateShortID(urlToShorten)
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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write(dst)
}

func (h *URLShortener) generateShortID(data string) ([]byte, error) {
	u, err := url.Parse(data)
	if err != nil || len(u.Hostname()) == 0 {
		return nil, errors.New("bad input data")
	}

	key, _, err := h.urlStorage.Add(u.String())
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
