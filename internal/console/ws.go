package console

import (
	"context"
	"encoding/json"

	"github.com/coder/websocket"
)

type WSConn struct {
	Conn *websocket.Conn
}

func (w *WSConn) Read(ctx context.Context) ([]byte, error) {
	_, data, err := w.Conn.Read(ctx)
	return data, err
}

func (w *WSConn) Write(ctx context.Context, data []byte) error {
	return w.Conn.Write(ctx, websocket.MessageBinary, data)
}

func (w *WSConn) Resize(ctx context.Context, width, height int) error {
	msg, _ := json.Marshal(map[string]int{"width": width, "height": height})
	return w.Conn.Write(ctx, websocket.MessageText, msg)
}

func (w *WSConn) Close() error {
	return w.Conn.Close(websocket.StatusNormalClosure, "")
}
