package console

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"golang.org/x/term"
)

type Conn interface {
	Read(ctx context.Context) ([]byte, error)
	Write(ctx context.Context, data []byte) error
	Resize(ctx context.Context, width, height int) error
	Close() error
}

func Run(ctx context.Context, conn Conn) error {
	fd := int(os.Stdin.Fd())
	width, height := 80, 24
	isTTY := term.IsTerminal(fd)
	if isTTY {
		if w, h, err := term.GetSize(fd); err == nil {
			width, height = w, h
		}
	}

	if err := conn.Resize(ctx, width, height); err != nil {
		return fmt.Errorf("setting terminal size: %w", err)
	}

	if isTTY {
		oldState, err := term.MakeRaw(fd)
		if err != nil {
			return fmt.Errorf("setting terminal raw mode: %w", err)
		}
		defer func() { _ = term.Restore(fd, oldState) }()
	}

	_, _ = fmt.Fprint(os.Stderr, "attached to console (press ctrl+c to detach)\r\n")

	consoleCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()
		for {
			data, err := conn.Read(consoleCtx)
			if err != nil {
				if consoleCtx.Err() == nil {
					_, _ = fmt.Fprintf(os.Stderr, "\r\ndetached from console\r\n")
				}
				return
			}
			if _, writeErr := os.Stdout.Write(data); writeErr != nil {
				return
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()
		buf := make([]byte, 4096)
		for {
			n, readErr := os.Stdin.Read(buf)
			for i := 0; i < n; i++ {
				if buf[i] == 0x03 {
					if i > 0 {
						_ = conn.Write(consoleCtx, buf[:i])
					}
					_ = conn.Close()
					return
				}
			}
			if n > 0 {
				if writeErr := conn.Write(consoleCtx, buf[:n]); writeErr != nil {
					return
				}
			}
			if readErr != nil {
				_ = conn.Close()
				return
			}
		}
	}()

	if isTTY {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(250 * time.Millisecond)
			defer ticker.Stop()
			lastW, lastH := width, height
			for {
				select {
				case <-consoleCtx.Done():
					return
				case <-ticker.C:
					w, h, err := term.GetSize(fd)
					if err != nil || (w == lastW && h == lastH) {
						continue
					}
					lastW, lastH = w, h
					_ = conn.Resize(consoleCtx, w, h)
				}
			}
		}()
	}

	wg.Wait()
	return nil
}
