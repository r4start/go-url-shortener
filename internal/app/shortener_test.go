package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

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

	h := NewURLShortener()
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

	h := NewURLShortener()
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

	h := NewURLShortener()
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
