package main

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"net"
	"net/http"
	"time"

	"github.com/Zyko0/go-sdl3/sdl"
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

// captureFramePNG must run on SDL's render thread, after drawing and before
// Present. It reads the renderer rather than the desktop, so window focus and
// occlusion do not affect the result.
func captureFramePNG(renderer *sdl.Renderer) ([]byte, error) {
	surface, err := renderer.ReadPixels(nil)
	if err != nil {
		return nil, fmt.Errorf("read renderer pixels: %w", err)
	}
	defer surface.Destroy()

	rgbaSurface := surface
	if surface.Format != sdl.PIXELFORMAT_RGBA32 {
		rgbaSurface, err = surface.Convert(sdl.PIXELFORMAT_RGBA32)
		if err != nil {
			return nil, fmt.Errorf("convert captured pixels to RGBA: %w", err)
		}
		defer rgbaSurface.Destroy()
	}
	if rgbaSurface.W <= 0 || rgbaSurface.H <= 0 || rgbaSurface.Pitch < rgbaSurface.W*4 {
		return nil, fmt.Errorf("invalid captured surface %dx%d pitch %d", rgbaSurface.W, rgbaSurface.H, rgbaSurface.Pitch)
	}

	width, height := int(rgbaSurface.W), int(rgbaSurface.H)
	frame := image.NewRGBA(image.Rect(0, 0, width, height))
	pixels := rgbaSurface.Pixels()
	pitch := int(rgbaSurface.Pitch)
	for y := 0; y < height; y++ {
		copy(frame.Pix[y*frame.Stride:y*frame.Stride+width*4], pixels[y*pitch:y*pitch+width*4])
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, frame); err != nil {
		return nil, fmt.Errorf("encode captured frame: %w", err)
	}
	return encoded.Bytes(), nil
}
