package main

import (
	"bufio"
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

func httpLog(f func(w http.ResponseWriter, r *http.Request)) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		startTime := time.Now()
		w2 := &httpResponseWriterCapturer{
			ResponseWriter: w,
		}
		ip := ""
		li := strings.LastIndex(r.RemoteAddr, ":")
		if li == -1 {
			ip = r.RemoteAddr
		} else {
			ip = r.RemoteAddr[:li]
		}
		cfip := r.Header.Get("CF-Connecting-IP")
		if cfip != "" {
			ip = cfip
		}
		auth := cfg.GetDString("", append([]string{"auth"}, r.Header.Get("Authorization"), "name")...)
		log.Info().Msgf("%15s %3d %7s %q %q %q",
			ip, w2.lastStatus, "hit",
			r.URL.RequestURI(), r.UserAgent(), auth)
		f(w2, r)
		log.Info().Msgf("%15s %3d %7s %q %q %q",
			ip, w2.lastStatus, time.Since(startTime).Round(1*time.Millisecond).String(),
			r.URL.RequestURI(), r.UserAgent(), auth)
	}
}

func httpRoutine(exitChan <-chan struct{}) {
	listenAddr := cfg.GetDString("0.0.0.0:40124", "http", "listenAddr")
	serv := http.Server{
		Addr:    listenAddr,
		Handler: makeHTTPServeMux(),
	}

	wg := &sync.WaitGroup{}
	wg.Go(func() {
		log.Info().Str("addr", listenAddr).Msg("http listening")
		err := serv.ListenAndServe()
		if !errors.Is(err, http.ErrServerClosed) {
			log.Err(err).Msg("server closed")
		}
	})

	<-exitChan

	ctx, ctxc := context.WithTimeout(context.Background(), 5*time.Second)
	defer ctxc()
	serv.Shutdown(ctx)
}

func handle404(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte("not found\n\n"))
}

func serveFile(name string, cache string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Cache-Control", cache)
		http.ServeFile(w, r, name)
	}
}

type httpResponseWriterCapturer struct {
	http.ResponseWriter
	lastStatus int
	http.Hijacker
	calledWrite  bool
	calledHeader bool
}

func (rc *httpResponseWriterCapturer) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return rc.ResponseWriter.(http.Hijacker).Hijack()
}

func (rc *httpResponseWriterCapturer) Unwrap() http.ResponseWriter {
	return rc.ResponseWriter
}

func (rc *httpResponseWriterCapturer) WriteHeader(statusCode int) {
	rc.lastStatus = statusCode
	rc.calledHeader = true
	rc.ResponseWriter.WriteHeader(statusCode)
}

func (rc *httpResponseWriterCapturer) Write(b []byte) (int, error) {
	if !rc.calledWrite && !rc.calledHeader {
		rc.lastStatus = 200
		rc.calledWrite = true
	}
	return rc.ResponseWriter.Write(b)
}
