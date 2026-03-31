package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"sync"

	"github.com/coder/websocket"
	"github.com/docker/docker/api/types/container"

	"github.com/oreforge/ore/internal/docker"
)

func Console(dockerClient docker.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		serverName := r.URL.Query().Get("server")
		if serverName == "" {
			WriteError(w, http.StatusBadRequest, "missing server query parameter")
			return
		}

		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()

		logger := slog.Default()

		_, msg, err := conn.Read(r.Context())
		if err != nil {
			logger.Error("console: reading initial size", "error", err)
			return
		}
		var size struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		}
		if err := json.Unmarshal(msg, &size); err != nil {
			logger.Error("console: parsing initial size", "error", err)
			return
		}

		hijacked, err := dockerClient.ContainerAttach(r.Context(), serverName, container.AttachOptions{
			Stream: true,
			Stdin:  true,
			Stdout: true,
			Stderr: true,
			Logs:   true,
		})
		if err != nil {
			logger.Error("console: attaching to container", "container", serverName, "error", err)
			return
		}
		defer hijacked.Close()

		if size.Width > 0 && size.Height > 0 {
			_ = dockerClient.ContainerResize(r.Context(), serverName, container.ResizeOptions{
				Width:  uint(size.Width),
				Height: uint(size.Height),
			})
		}

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()

		var wg sync.WaitGroup

		wg.Add(1)
		go func() {
			defer wg.Done()
			defer cancel()
			buf := make([]byte, 4096)
			for {
				n, readErr := hijacked.Conn.Read(buf)
				if n > 0 {
					if writeErr := conn.Write(ctx, websocket.MessageBinary, buf[:n]); writeErr != nil {
						return
					}
				}
				if readErr != nil {
					_ = conn.Close(websocket.StatusNormalClosure, "container process exited")
					return
				}
			}
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			defer cancel()
			for {
				typ, data, readErr := conn.Read(ctx)
				if readErr != nil {
					_ = hijacked.CloseWrite()
					return
				}
				if typ == websocket.MessageText {
					var resize struct {
						Width  int `json:"width"`
						Height int `json:"height"`
					}
					if json.Unmarshal(data, &resize) == nil && resize.Width > 0 && resize.Height > 0 {
						_ = dockerClient.ContainerResize(ctx, serverName, container.ResizeOptions{
							Width:  uint(resize.Width),
							Height: uint(resize.Height),
						})
					}
					continue
				}
				if _, err := io.Copy(hijacked.Conn, bytes.NewReader(data)); err != nil {
					return
				}
			}
		}()

		wg.Wait()
	}
}
