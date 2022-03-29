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
			name: "patch check",
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

			assert.Equal(t, tt.expectedCode, result.StatusCode)
		})
	}
}

func TestURLShortener_getURL(t *testing.T) {
	type args struct {
		w            *httptest.ResponseRecorder
		r            *http.Request
		expectedCode int
	}
	tests := []struct {
		name string
		args []args
	}{
		{
			name: "Shortener check #1",
			args: []args{
				{
					w:            httptest.NewRecorder(),
					r:            httptest.NewRequest(http.MethodPost, "/", strings.NewReader("https://ya.ru")),
					expectedCode: http.StatusCreated,
				},
				{
					w:            httptest.NewRecorder(),
					r:            httptest.NewRequest(http.MethodGet, "/17627783340430073139", nil),
					expectedCode: http.StatusTemporaryRedirect,
				},
			},
		},
		{
			name: "Shortener check #2",
			args: []args{
				{
					w:            httptest.NewRecorder(),
					r:            httptest.NewRequest(http.MethodPost, "/", strings.NewReader("https://vc.ru")),
					expectedCode: http.StatusCreated,
				},
				{
					w:            httptest.NewRecorder(),
					r:            httptest.NewRequest(http.MethodGet, "/4506343413788829418", strings.NewReader("https://vc.ru")),
					expectedCode: http.StatusTemporaryRedirect,
				},
			},
		},
	}

	h := NewURLShortener()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, a := range tt.args {
				h.ServeHTTP(a.w, a.r)
				result := a.w.Result()

				assert.Equal(t, a.expectedCode, result.StatusCode)
			}
		})
	}
}

func TestURLShortener_shorten(t *testing.T) {
	type args struct {
		w *httptest.ResponseRecorder
		r *http.Request
	}
	tests := []struct {
		name     string
		args     args
		expected string
	}{
		{
			name: "Shortener check #1",
			args: args{
				w: httptest.NewRecorder(),
				r: httptest.NewRequest(http.MethodPost, "/", strings.NewReader("https://ya.ru")),
			},
			expected: "http://example.com/17627783340430073139",
		},
		{
			name: "Shortener check #2",
			args: args{
				w: httptest.NewRecorder(),
				r: httptest.NewRequest(http.MethodPost, "/", strings.NewReader("https://vc.ru")),
			},
			expected: "http://example.com/4506343413788829418",
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

			assert.Equal(t, tt.expected, string(resBody))
		})
	}
}
