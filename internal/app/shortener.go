package app

import (
	"encoding/base64"
	"fmt"
	"github.com/go-chi/chi/v5"
	"hash/fnv"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
)

type URLShortener struct {
	*chi.Mux
	urls map[uint64]*url.URL
	lock sync.RWMutex
}

func NewURLShortener() *URLShortener {
	handler := &URLShortener{Mux: chi.NewMux(), urls: make(map[uint64]*url.URL), lock: sync.RWMutex{}}
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
	if err != nil {
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	hasher := fnv.New64()
	_, err = hasher.Write(b)
	if err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	key := hasher.Sum64()

	h.lock.Lock()
	h.urls[key] = u
	h.lock.Unlock()

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
	var u url.URL
	h.lock.RLock()
	if v, ok := h.urls[key]; ok {
		u = *v
	} else {
		h.lock.RUnlock()
		http.Error(w, "", http.StatusNotFound)
		return
	}
	h.lock.RUnlock()

	w.Header().Set("Location", u.String())
	w.WriteHeader(http.StatusTemporaryRedirect)
}
