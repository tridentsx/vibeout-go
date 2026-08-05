package main

import (
	"net"
	"net/http"
	"time"
)

type frameCaptureResult struct {
	png []byte
	err error
}

type frameCaptureRequest struct {
	result chan frameCaptureResult
}

type frameServer struct {
	requests chan frameCaptureRequest
	server   *http.Server
}

func startFrameServer(address string) (*frameServer, error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	frames := &frameServer{requests: make(chan frameCaptureRequest)}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /frame.png", frames.handleFrame)
	frames.server = &http.Server{Handler: mux, ReadHeaderTimeout: 2 * time.Second}
	go func() { _ = frames.server.Serve(listener) }()
	return frames, nil
}

func (s *frameServer) Close() error {
	return s.server.Close()
}

func (s *frameServer) handleFrame(w http.ResponseWriter, r *http.Request) {
	request := frameCaptureRequest{result: make(chan frameCaptureResult, 1)}
	select {
	case s.requests <- request:
	case <-r.Context().Done():
		return
	case <-time.After(5 * time.Second):
		http.Error(w, "renderer did not accept capture request", http.StatusServiceUnavailable)
		return
	}
	select {
	case result := <-request.result:
		if result.err != nil {
			http.Error(w, result.err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(result.png)
	case <-r.Context().Done():
	case <-time.After(5 * time.Second):
		http.Error(w, "frame capture timed out", http.StatusGatewayTimeout)
	}
}
