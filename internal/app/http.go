package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/r4start/go-url-shortener/pkg/storage"
	"go.uber.org/zap"
)

const (
	UserIDCookieName = "gusid"
)

var ErrBadRequest = errors.New("bad request")

type apiRequestData struct {
	UserID        uint64
	IsIDGenerated bool
}

type deleteData struct {
	UserID uint64
	IDs    []string
}

type HTTPServer struct {
	*chi.Mux
	shortener  *URLShortener
	domain     string
	logger     *zap.Logger
	trustedNet *net.IPNet
}

func NewHTTPServer(shortener *URLShortener, logger *zap.Logger, opts ...HTTPServerConfigurator) (*HTTPServer, error) {
	handler := &HTTPServer{
		Mux:       chi.NewMux(),
		shortener: shortener,
		logger:    logger,
	}

	for _, o := range opts {
		o(handler)
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

	handler.Get("/api/internal/stats", handler.apiInternalStats)

	handler.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "", http.StatusBadRequest)
	})

	return handler, nil
}

func (s *HTTPServer) shorten(w http.ResponseWriter, r *http.Request) {
	userID, generated, err := s.getUserID(r)
	if err != nil {
		s.logger.Error("failed to generate user id", zap.Error(err))
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	b, err := io.ReadAll(r.Body)
	if err != nil {
		s.logger.Error("failed to read request body", zap.Error(err))
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	res, err := s.shortener.Shorten(r.Context(), userID, string(b))
	if err != nil {
		s.logger.Error("failed to generate short id", zap.Error(err))
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	if generated {
		if err := s.setUserID(w, userID); err != nil {
			s.logger.Error("failed to set user id", zap.Error(err))
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
	}

	if !res.Exists {
		w.WriteHeader(http.StatusCreated)
	} else {
		w.WriteHeader(http.StatusConflict)
	}

	if _, err := w.Write([]byte(s.makeResultURL(r, res.Key))); err != nil {
		s.logger.Error("failed to write response body", zap.Error(err))
	}
}

func (s *HTTPServer) getURL(w http.ResponseWriter, r *http.Request) {
	keyData := chi.URLParam(r, "id")

	u, err := s.shortener.OriginalURL(r.Context(), keyData)
	if err == storage.ErrDeleted {
		w.WriteHeader(http.StatusGone)
		return
	} else if err != nil {
		s.logger.Error("failed to get original url", zap.Error(err))
		http.Error(w, "", http.StatusNotFound)
		return
	}

	w.Header().Set("Location", u)
	w.WriteHeader(http.StatusTemporaryRedirect)
}

func (s *HTTPServer) apiShortener(w http.ResponseWriter, r *http.Request) {
	var request map[string]string

	reqData, err := s.apiParseRequest(r, &request)
	if errors.Is(err, ErrBadRequest) {
		http.Error(w, "", http.StatusBadRequest)
		return
	} else if err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	urlToShorten, ok := request["url"]
	if !ok {
		s.logger.Error("empty url in request body")
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	res, err := s.shortener.Shorten(r.Context(), reqData.UserID, urlToShorten)
	if err != nil {
		s.logger.Error("failed to generate short id", zap.Error(err))
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	response := make(map[string]string)
	response["result"] = s.makeResultURL(r, res.Key)

	statusCode := http.StatusCreated
	if res.Exists {
		statusCode = http.StatusConflict
	}
	s.apiWriteResponse(w, reqData, statusCode, response)
}

func (s *HTTPServer) apiBatchShortener(w http.ResponseWriter, r *http.Request) {
	type request struct {
		CorrelationID string `json:"correlation_id"`
		OriginalURL   string `json:"original_url"`
	}

	type response struct {
		CorrelationID string `json:"correlation_id"`
		ShortURL      string `json:"short_url"`
	}

	requestData := make([]request, 0)
	reqData, err := s.apiParseRequest(r, &requestData)
	if errors.Is(err, ErrBadRequest) {
		s.logger.Error("bad request", zap.Error(err))
		http.Error(w, "", http.StatusBadRequest)
		return
	} else if err != nil {
		s.logger.Error("failed to parse request", zap.Error(err))
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	urls := make([]string, 0, len(requestData))
	for _, e := range requestData {
		urls = append(urls, e.OriginalURL)
	}

	encodedIds, err := s.shortener.BatchShorten(r.Context(), reqData.UserID, urls)
	if err != nil {
		s.logger.Error("failed to generate short ids", zap.Error(err))
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	responseData := make([]response, 0, len(encodedIds))
	for i, dst := range encodedIds {
		responseData = append(responseData, response{
			CorrelationID: requestData[i].CorrelationID,
			ShortURL:      s.makeResultURL(r, dst),
		})
	}

	s.apiWriteResponse(w, reqData, http.StatusCreated, responseData)
}

func (s *HTTPServer) apiUserURLs(w http.ResponseWriter, r *http.Request) {
	userID, generated, err := s.getUserID(r)
	if err != nil {
		s.logger.Error("failed to generate user id", zap.Error(err))
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	if generated {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	userUrls, err := s.shortener.UserURLs(r.Context(), userID)
	if err != nil {
		s.logger.Error("failed to get user data", zap.Error(err))
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
			ShortURL:    s.makeResultURL(r, EncodeID(u.ShortURLID)),
			OriginalURL: u.OriginalURL,
		})
	}

	s.apiWriteResponse(w, &apiRequestData{
		UserID: userID,
	}, http.StatusOK, result)
}

func (s *HTTPServer) apiDeleteUserURLs(w http.ResponseWriter, r *http.Request) {
	requestData := make([]string, 0)
	reqData, err := s.apiParseRequest(r, &requestData)
	if errors.Is(err, ErrBadRequest) {
		s.logger.Error("bad request", zap.Error(err))
		http.Error(w, "", http.StatusBadRequest)
		return
	} else if err != nil {
		s.logger.Error("failed to parse request", zap.Error(err))
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	if reqData.IsIDGenerated {
		s.logger.Error("unknown user id")
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	if err := s.shortener.DeleteUserURLs(r.Context(), reqData.UserID, requestData); err != nil {
		s.logger.Error("failed to delete user urls", zap.Error(err))
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

func (s *HTTPServer) ping(w http.ResponseWriter, r *http.Request) {
	if err := s.shortener.Ping(r.Context()); err != nil {
		s.logger.Error("failed to ping shortener", zap.Error(err))
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *HTTPServer) apiInternalStats(w http.ResponseWriter, r *http.Request) {
	type response struct {
		URLs  uint64 `json:"urls"`
		Users uint64 `json:"users"`
	}

	realIP := r.Header.Get("x-real-ip")
	userIP := net.ParseIP(realIP)
	if userIP == nil || s.trustedNet == nil || !s.trustedNet.Contains(userIP) {
		http.Error(w, "", http.StatusForbidden)
		return
	}

	stat, err := s.shortener.Stat(r.Context())
	if err != nil {
		s.logger.Error("failed to get stats", zap.Error(err))
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	resp := response{
		URLs:  stat.URLs,
		Users: stat.Users,
	}

	s.apiWriteResponse(w, nil /*apiRequestData*/, http.StatusOK, resp)
}

func (s *HTTPServer) makeResultURL(r *http.Request, data []byte) string {
	if len(s.domain) != 0 {
		return fmt.Sprintf("%s/%s", s.domain, string(data))
	}

	protocol := "http"
	if r.TLS != nil {
		protocol = "https"
	}

	return fmt.Sprintf("%s://%s/%s", protocol, r.Host, string(data))
}

func (s *HTTPServer) apiParseRequest(r *http.Request, body interface{}) (*apiRequestData, error) {
	if contentType := r.Header.Get("Content-Type"); contentType != "application/json" {
		s.logger.Error("bad content type", zap.String("content_type", contentType))
		return nil, ErrBadRequest
	}

	userID, generated, err := s.getUserID(r)
	if err != nil {
		s.logger.Error("failed to generate user id", zap.Error(err))
		return nil, err
	}

	b, err := io.ReadAll(r.Body)
	if err != nil {
		s.logger.Error("failed to read request body", zap.Error(err))
		return nil, err
	}

	if err = json.Unmarshal(b, &body); err != nil {
		s.logger.Error("failed to unmarshal request json", zap.Error(err))
		return nil, ErrBadRequest
	}
	return &apiRequestData{
		UserID:        userID,
		IsIDGenerated: generated,
	}, nil
}

func (s *HTTPServer) apiWriteResponse(w http.ResponseWriter, reqData *apiRequestData, statusCode int, response interface{}) {
	dst, err := json.Marshal(response)
	if err != nil {
		s.logger.Error("failed to marshal response", zap.Error(err))
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	if reqData != nil && reqData.IsIDGenerated {
		if err := s.setUserID(w, reqData.UserID); err != nil {
			s.logger.Error("failed to set user id", zap.Error(err))
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if _, err := w.Write(dst); err != nil {
		s.logger.Error("failed to write response body", zap.Error(err))
	}
}

func (s *HTTPServer) getUserID(r *http.Request) (uint64, bool, error) {
	userIDCookie, err := r.Cookie(UserIDCookieName)
	if err == http.ErrNoCookie {
		return s.shortener.GetUserID(nil)
	} else if err != nil {
		return 0, false, err
	}

	return s.shortener.GetUserID(&userIDCookie.Value)
}

func (s *HTTPServer) setUserID(w http.ResponseWriter, userID uint64) error {
	cookieValue, err := s.shortener.GenerateUserID(userID)
	if err != nil {
		return err
	}

	w.Header().Set("set-cookie", fmt.Sprintf(`%s=%s; Path=/`, UserIDCookieName, *cookieValue))

	return nil
}
