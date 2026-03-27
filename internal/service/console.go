package service

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"

	"github.com/oreforge/ore/internal/docker"
	"github.com/oreforge/ore/internal/spec"
)

func startConsoleProxy(specPath, listenHost, serverName string, replica int) (string, string, error) {
	s, err := spec.Load(specPath)
	if err != nil {
		return "", "", err
	}

	var srv *spec.ServerSpec
	for i := range s.Servers {
		if s.Servers[i].Name == serverName {
			srv = &s.Servers[i]
			break
		}
	}
	if srv == nil {
		return "", "", fmt.Errorf("server %q not found", serverName)
	}

	containerName := serverName
	if srv.EffectiveReplicas() > 1 {
		containerName = fmt.Sprintf("%s-%d", serverName, replica)
	}

	var nonceBuf [16]byte
	if _, err := rand.Read(nonceBuf[:]); err != nil {
		return "", "", fmt.Errorf("generating nonce: %w", err)
	}
	nonce := hex.EncodeToString(nonceBuf[:])

	bindAddr := listenHost + ":0"
	if listenHost == "" {
		bindAddr = ":0"
	}

	lis, err := net.Listen("tcp", bindAddr)
	if err != nil {
		return "", "", fmt.Errorf("listening for console: %w", err)
	}

	tcpLis, ok := lis.(*net.TCPListener)
	if !ok {
		_ = lis.Close()
		return "", "", fmt.Errorf("unexpected listener type")
	}
	_ = tcpLis.SetDeadline(time.Now().Add(10 * time.Second))

	addr := lis.Addr().String()
	logger := slog.Default()

	go func() {
		defer func() { _ = lis.Close() }()

		conn, acceptErr := lis.Accept()
		if acceptErr != nil {
			logger.Debug("console proxy accept timeout", "error", acceptErr)
			return
		}

		handleConsoleConn(conn, containerName, nonce, logger)
	}()

	return addr, nonce, nil
}

func handleConsoleConn(conn net.Conn, containerName, expectedNonce string, logger *slog.Logger) {
	defer func() { _ = conn.Close() }()

	nonceBuf := make([]byte, len(expectedNonce))
	if _, err := io.ReadFull(conn, nonceBuf); err != nil {
		logger.Error("console: reading nonce", "error", err)
		return
	}
	if string(nonceBuf) != expectedNonce {
		logger.Warn("console: invalid nonce")
		return
	}

	var sizeBuf [4]byte
	if _, err := io.ReadFull(conn, sizeBuf[:]); err != nil {
		logger.Error("console: reading terminal size", "error", err)
		return
	}
	width := binary.BigEndian.Uint16(sizeBuf[0:2])
	height := binary.BigEndian.Uint16(sizeBuf[2:4])

	dockerClient, err := docker.New(context.Background())
	if err != nil {
		logger.Error("console: connecting to Docker", "error", err)
		return
	}
	defer func() { _ = dockerClient.Close() }()

	if width > 0 && height > 0 {
		_ = dockerClient.ContainerResize(context.Background(), containerName, container.ResizeOptions{
			Width:  uint(width),
			Height: uint(height),
		})
	}

	hijacked, err := dockerClient.ContainerAttach(context.Background(), containerName, container.AttachOptions{
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

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(conn, hijacked.Conn)
		_ = conn.(*net.TCPConn).CloseWrite()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(hijacked.Conn, conn)
		_ = hijacked.CloseWrite()
	}()

	wg.Wait()
}
