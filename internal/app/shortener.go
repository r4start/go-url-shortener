package app

import (
	"encoding/base64"
	"fmt"
	"github.com/go-chi/chi/v5"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/r4start/go-url-shortener/internal/storage"
)

type URLShortener struct {
	*chi.Mux
	urlStorage storage.URLStorage
}

func NewURLShortener() *URLShortener {
	handler := &URLShortener{Mux: chi.NewMux(), urlStorage: storage.NewInMemoryStorage()}
	handler.Get("/{id}", handler.getURL)
	handler.Post("/", handler.shorten)

	handler.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "", http.StatusBadRequest)
	})

	return handler
}

func (h *URLShortener) shorten(w http.ResponseWriter, r *http.Request) {
	b, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	u, err := url.Parse(string(b))
	if err != nil || len(u.Hostname()) == 0 {
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	key, err := h.urlStorage.Add(u.String())
	if err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	keyData := []byte(strconv.FormatUint(key, 16))
	dst := make([]byte, base64.RawURLEncoding.EncodedLen(len(keyData)))
	base64.RawURLEncoding.Encode(dst, keyData)

	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(fmt.Sprintf("http://%s/%s", r.Host, string(dst))))
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
