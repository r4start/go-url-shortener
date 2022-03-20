package app

import (
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
)

type URLShortener struct {
	urls map[uint64]*url.URL
	lock sync.RWMutex
}

func NewURLShortener() *URLShortener {
	return &URLShortener{urls: make(map[uint64]*url.URL), lock: sync.RWMutex{}}
}

func (h *URLShortener) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.getURL(w, r)
	case http.MethodPost:
		h.shorten(w, r)
	default:
		http.Error(w, "", http.StatusBadRequest)
	}
}

func (h *URLShortener) shorten(w http.ResponseWriter, r *http.Request) {
	if r.URL.RequestURI() != "/" {
		http.Error(w, "", http.StatusBadRequest)
		return
	}

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
	w.Write([]byte(fmt.Sprintf("http://localhost:8080/%d\r\n", key)))
}

func (h *URLShortener) getURL(w http.ResponseWriter, r *http.Request) {
	key, err := strconv.ParseUint(r.URL.RequestURI()[1:], 10, 64)
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
