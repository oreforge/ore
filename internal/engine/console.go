package engine

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"sync"

	"golang.org/x/term"
)

func rawConsole(addr, nonce string) error {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("connecting to console: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.Write([]byte(nonce)); err != nil {
		return fmt.Errorf("sending nonce: %w", err)
	}

	fd := int(os.Stdin.Fd())
	width, height := 80, 24
	isTTY := term.IsTerminal(fd)
	if isTTY {
		w, h, sizeErr := term.GetSize(fd)
		if sizeErr == nil {
			width, height = w, h
		}
	}

	var sizeBuf [4]byte
	binary.BigEndian.PutUint16(sizeBuf[0:2], uint16(width))
	binary.BigEndian.PutUint16(sizeBuf[2:4], uint16(height))
	if _, err := conn.Write(sizeBuf[:]); err != nil {
		return fmt.Errorf("sending terminal size: %w", err)
	}

	if isTTY {
		oldState, termErr := term.MakeRaw(fd)
		if termErr != nil {
			return fmt.Errorf("setting terminal raw mode: %w", termErr)
		}
		defer func() { _ = term.Restore(fd, oldState) }()
	}

	_, _ = fmt.Fprint(os.Stderr, "attached to console (press ctrl+c to detach)\r\n")
	_, _ = conn.Write([]byte("\n"))

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(os.Stdout, conn)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 256)
		for {
			n, readErr := os.Stdin.Read(buf)
			for i := 0; i < n; i++ {
				if buf[i] == 0x03 {
					if i > 0 {
						_, _ = conn.Write(buf[:i])
					}
					_ = conn.Close()
					return
				}
			}
			if n > 0 {
				if _, writeErr := conn.Write(buf[:n]); writeErr != nil {
					return
				}
			}
			if readErr != nil {
				_ = conn.Close()
				return
			}
		}
	}()

	wg.Wait()
	return nil
}
