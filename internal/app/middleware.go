package app

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
)

type gzipBodyReader struct {
	gzipReader *gzip.Reader
}

func (gz *gzipBodyReader) Read(p []byte) (n int, err error) {
	return gz.gzipReader.Read(p)
}

func (gz *gzipBodyReader) Close() error {
	return gz.gzipReader.Close()
}

func UnpackGzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Encoding") == "gzip" {
			gz, err := gzip.NewReader(r.Body)
			if err != nil {
				http.Error(w, "", http.StatusInternalServerError)
				return
			}
			r.Body = &gzipBodyReader{gzipReader: gz}
		}
		next.ServeHTTP(w, r)
	})
}

type gzipBodyWriter struct {
	http.ResponseWriter
	writer io.Writer
}

func (gz gzipBodyWriter) Write(b []byte) (int, error) {
	return gz.writer.Write(b)
}

func CompressGzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(r.Header.Get("Accept-Encoding")))
		//gz, err := gzip.NewWriterLevel(w, gzip.BestCompression)
		//if err != nil {
		//	http.Error(w, "", http.StatusInternalServerError)
		//	return
		//}
		//defer gz.Close()
		//
		//next.ServeHTTP(gzipBodyWriter{
		//	ResponseWriter: w,
		//	writer:         gz,
		//}, r)
	})
}
