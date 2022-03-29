package app

import (
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"

	"github.com/go-chi/chi/v5"
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

	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(fmt.Sprintf("http://%s/%d", r.Host, key)))
}

func (h *URLShortener) getURL(w http.ResponseWriter, r *http.Request) {
	key, err := strconv.ParseUint(chi.URLParam(r, "id"), 10, 64)
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
