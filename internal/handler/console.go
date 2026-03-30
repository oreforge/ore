package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"sync"

	"github.com/coder/websocket"
	"github.com/docker/docker/api/types/container"

	"github.com/oreforge/ore/internal/docker"
	"github.com/oreforge/ore/internal/spec"
)

func Console() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		specPath := SpecPathFromCtx(r.Context())
		serverName := r.URL.Query().Get("server")
		if serverName == "" {
			WriteError(w, http.StatusBadRequest, "missing server query parameter")
			return
		}
		replica, _ := strconv.Atoi(r.URL.Query().Get("replica"))
		if replica == 0 {
			replica = 1
		}

		s, err := spec.Load(specPath)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}

		var srv *spec.ServerSpec
		for i := range s.Servers {
			if s.Servers[i].Name == serverName {
				srv = &s.Servers[i]
				break
			}
		}
		if srv == nil {
			WriteError(w, http.StatusNotFound, "server "+serverName+" not found")
			return
		}

		containerName := serverName
		if srv.EffectiveReplicas() > 1 {
			containerName = serverName + "-" + strconv.Itoa(replica)
		}

		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()

		logger := slog.Default()

		_, msg, err := conn.Read(r.Context())
		if err != nil {
			logger.Error("console: reading terminal size", "error", err)
			return
		}
		var size struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		}
		if err := json.Unmarshal(msg, &size); err != nil {
			logger.Error("console: parsing terminal size", "error", err)
			return
		}

		dockerClient, err := docker.New(r.Context())
		if err != nil {
			logger.Error("console: connecting to Docker", "error", err)
			return
		}
		defer func() { _ = dockerClient.Close() }()

		hijacked, err := dockerClient.ContainerAttach(r.Context(), containerName, container.AttachOptions{
			Stream: true,
			Stdin:  true,
			Stdout: true,
			Stderr: true,
		})
		if err != nil {
			logger.Error("console: attaching to container", "container", containerName, "error", err)
			return
		}
		defer hijacked.Close()

		if size.Width > 0 && size.Height > 0 {
			_ = dockerClient.ContainerResize(r.Context(), containerName, container.ResizeOptions{
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
					var ctrl struct {
						Resize bool `json:"resize"`
						Width  int  `json:"width"`
						Height int  `json:"height"`
					}
					if json.Unmarshal(data, &ctrl) == nil && ctrl.Resize {
						_ = dockerClient.ContainerResize(ctx, containerName, container.ResizeOptions{
							Width:  uint(ctrl.Width),
							Height: uint(ctrl.Height),
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
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}
}
