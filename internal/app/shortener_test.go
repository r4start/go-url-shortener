package app

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/r4start/go-url-shortener/internal/storage"
	"go.uber.org/zap"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func testShortener(t *testing.T) *URLShortener {
	logger, _ := zap.NewDevelopment()
	h, err := NewURLShortener(nil, "", storage.NewInMemoryStorage(), logger)
	assert.Nil(t, err)
	return h
}

func TestURLShortener_ServeHTTP(t *testing.T) {
	type args struct {
		w *httptest.ResponseRecorder
		r *http.Request
	}
	tests := []struct {
		name         string
		args         args
		expectedCode int
	}{
		{
			name: "Invalid method check #1",
			args: args{
				w: httptest.NewRecorder(),
				r: httptest.NewRequest(http.MethodPatch, "/", nil),
			},
			expectedCode: http.StatusBadRequest,
		},
	}

	h := testShortener(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h.ServeHTTP(tt.args.w, tt.args.r)
			result := tt.args.w.Result()
			defer result.Body.Close()

			assert.Equal(t, tt.expectedCode, result.StatusCode)
		})
	}
}

func TestURLShortener_getURL(t *testing.T) {
	tests := []struct {
		name     string
		checkURL string
	}{
		{
			name:     "Shortener check #1",
			checkURL: "https://ya.ru",
		},
		{
			name:     "Shortener check #2",
			checkURL: "https://vc.ru",
		},
		{
			name:     "Shortener check #3",
			checkURL: "https://lenta.com.ru",
		},
	}

	h := testShortener(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.checkURL))

			h.ServeHTTP(w, r)
			result := w.Result()

			defer result.Body.Close()
			resBody, err := io.ReadAll(result.Body)
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, http.StatusCreated, result.StatusCode)

			w = httptest.NewRecorder()
			r = httptest.NewRequest(http.MethodGet, string(resBody), nil)

			h.ServeHTTP(w, r)
			result = w.Result()
			defer result.Body.Close()

			assert.Equal(t, http.StatusTemporaryRedirect, result.StatusCode)
			assert.Equal(t, tt.checkURL, result.Header.Get("Location"))
		})
	}
}

func TestURLShortener_shorten(t *testing.T) {
	type args struct {
		w *httptest.ResponseRecorder
		r *http.Request
	}
	type expected struct {
		expectedCode int
		expectedURL  string
	}
	tests := []struct {
		name string
		args args
		expected
	}{
		{
			name: "Shortener check #1",
			args: args{
				w: httptest.NewRecorder(),
				r: httptest.NewRequest(http.MethodPost, "/", strings.NewReader("https://ya.ru")),
			},
			expected: expected{
				expectedCode: http.StatusCreated,
				expectedURL:  "http://example.com/ZjRhMjc3OGQ1N2UyMWQzMw",
			},
		},
		{
			name: "Shortener check #2",
			args: args{
				w: httptest.NewRecorder(),
				r: httptest.NewRequest(http.MethodPost, "/", strings.NewReader("https://vc.ru")),
			},
			expected: expected{
				expectedCode: http.StatusCreated,
				expectedURL:  "http://example.com/M2U4OWJmNzU4ZWNkZTZlYQ",
			},
		},
		{
			name: "Shortener check #3",
			args: args{
				w: httptest.NewRecorder(),
				r: httptest.NewRequest(http.MethodPost, "/", strings.NewReader("vc.ru")),
			},
			expected: expected{
				expectedCode: http.StatusBadRequest,
				expectedURL:  "",
			},
		},
		{
			name: "Shortener check #4",
			args: args{
				w: httptest.NewRecorder(),
				r: httptest.NewRequest(http.MethodPost, "/", strings.NewReader("kajsdhkashd")),
			},
			expected: expected{
				expectedCode: http.StatusBadRequest,
				expectedURL:  "",
			},
		},
		{
			name: "Shortener check #5",
			args: args{
				w: httptest.NewRecorder(),
				r: httptest.NewRequest(http.MethodPost, "/", strings.NewReader("")),
			},
			expected: expected{
				expectedCode: http.StatusBadRequest,
				expectedURL:  "",
			},
		},
	}

	h := testShortener(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h.ServeHTTP(tt.args.w, tt.args.r)
			result := tt.args.w.Result()

			defer result.Body.Close()
			resBody, err := io.ReadAll(result.Body)
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, tt.expectedCode, result.StatusCode)
			if result.StatusCode == http.StatusCreated {
				assert.Equal(t, tt.expectedURL, string(resBody))
			}
		})
	}
}

func TestURLShortener_apiShortener(t *testing.T) {
	type args struct {
		w *httptest.ResponseRecorder
		r *http.Request
	}
	type expected struct {
		expectedCode     int
		expectedResponse string
	}
	tests := []struct {
		name string
		args args
		expected
	}{
		{
			name: "Shortener check #1",
			args: args{
				w: httptest.NewRecorder(),
				r: httptest.NewRequest(http.MethodPost, "/api/shorten", strings.NewReader(`{"url":"http://ya.ru"}`)),
			},
			expected: expected{
				expectedCode:     http.StatusCreated,
				expectedResponse: `{"result":"http://example.com/ZDIyNDk4MzQzMGZmMDQ1ZQ"}`,
			},
		},
		{
			name: "Shortener check #2",
			args: args{
				w: httptest.NewRecorder(),
				r: httptest.NewRequest(http.MethodPost, "/api/shorten", strings.NewReader(`{"url":"http://vc.ru"}`)),
			},
			expected: expected{
				expectedCode:     http.StatusCreated,
				expectedResponse: `{"result":"http://example.com/NWI4NTMwNmZjNWJmMjMzYg"}`,
			},
		},
		{
			name: "Shortener check #3",
			args: args{
				w: httptest.NewRecorder(),
				r: httptest.NewRequest(http.MethodPost, "/api/shorten", strings.NewReader("vc.ru")),
			},
			expected: expected{
				expectedCode:     http.StatusBadRequest,
				expectedResponse: "",
			},
		},
		{
			name: "Shortener check #4",
			args: args{
				w: httptest.NewRecorder(),
				r: httptest.NewRequest(http.MethodPost, "/api/shorten", strings.NewReader("kajsdhkashd")),
			},
			expected: expected{
				expectedCode:     http.StatusBadRequest,
				expectedResponse: "",
			},
		},
		{
			name: "Shortener check #5",
			args: args{
				w: httptest.NewRecorder(),
				r: httptest.NewRequest(http.MethodPost, "/api/shorten", strings.NewReader("")),
			},
			expected: expected{
				expectedCode:     http.StatusBadRequest,
				expectedResponse: "",
			},
		},
	}

	h := testShortener(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.args.r.Header.Set("content-type", "application/json")
			h.ServeHTTP(tt.args.w, tt.args.r)
			result := tt.args.w.Result()

			defer result.Body.Close()
			resBody, err := io.ReadAll(result.Body)
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, tt.expectedCode, result.StatusCode)
			if result.StatusCode == http.StatusCreated {
				assert.Equal(t, tt.expectedResponse, string(resBody))
			}
		})
	}
}

func TestURLShortener_apiBatchShortener(t *testing.T) {
	type request struct {
		CorrelationID string `json:"correlation_id"`
		OriginalURL   string `json:"original_url"`
	}

	type args struct {
		URLs []string
	}
	type expected struct {
		expectedCode     int
		expectedResponse string
	}
	tests := []struct {
		name string
		args args
		expected
	}{
		{
			name: "Shortener check #1",
			args: args{
				URLs: []string{
					"http://ya.ru",
					"http://vc.ru",
					"http://habr.ru",
					"http://lenta.ru",
					"http://vk.ru",
					"http://ok.ru",
					"http://vz.ru",
					"http://ria.ru",
					"http://goog.le",
				},
			},
			expected: expected{
				expectedCode: http.StatusCreated,
				expectedResponse: `[{"correlation_id":"0","short_url":"http://example.com/ZDIyNDk4MzQzMGZmMDQ1ZQ"},` +
					`{"correlation_id":"1","short_url":"http://example.com/NWI4NTMwNmZjNWJmMjMzYg"},` +
					`{"correlation_id":"2","short_url":"http://example.com/NGViNTExNTZlMzI2NmNiMw"},` +
					`{"correlation_id":"3","short_url":"http://example.com/ZTdjMTdjZDVlMTY3YjQ1YQ"},` +
					`{"correlation_id":"4","short_url":"http://example.com/YTE3MzY4NmZlZDg4NmE2Mw"},` +
					`{"correlation_id":"5","short_url":"http://example.com/MWE5MGMyYWI3OTVmNDRjZQ"},` +
					`{"correlation_id":"6","short_url":"http://example.com/MWZlNTFiNmZhNDQyOWNiOA"},` +
					`{"correlation_id":"7","short_url":"http://example.com/N2NlNjg3NzEyMzQzZGNlZQ"},` +
					`{"correlation_id":"8","short_url":"http://example.com/N2YwNTlmY2E2NGNlZWJjZQ"}]`,
			},
		},
	}

	h := testShortener(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requests := make([]request, 0)
			for i, u := range tt.args.URLs {
				requests = append(requests, request{
					CorrelationID: strconv.FormatUint(uint64(i), 10),
					OriginalURL:   u,
				})
			}

			body, err := json.Marshal(requests)
			assert.Nil(t, err)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/api/shorten/batch", bytes.NewReader(body))
			r.Header.Set("content-type", "application/json")
			h.ServeHTTP(w, r)
			result := w.Result()

			defer result.Body.Close()
			resBody, err := io.ReadAll(result.Body)
			assert.Nil(t, err)

			assert.Equal(t, tt.expectedCode, result.StatusCode)
			assert.Equal(t, tt.expectedResponse, string(resBody))
		})
	}
}

func Test_batchDecodeIDs(t *testing.T) {
	ids := make([]string, 5000)
	for i := 0; i < len(ids); i++ {
		ids[i] = "NWI4NTMwNmZjNWJmMjMzYg"
	}

	t.Run("Batch decode test", func(t *testing.T) {
		decodedIds, err := batchDecodeIDs(context.Background(), ids)
		assert.Nil(t, err)
		assert.Equal(t, len(ids), len(decodedIds))
	})
}
