package traffic

import (
	"context"
	"encoding/json"
	"io"

	coderws "github.com/coder/websocket"
)

// ReadWS counts decompressed message bytes as they are consumed, including
// partial messages before cancellation. Control frames are deliberately excluded.
func ReadWS(ctx context.Context, c *coderws.Conn, m *Meter, d int) (coderws.MessageType, []byte, error) {
	kind, r, err := c.Reader(ctx)
	if err != nil {
		return kind, nil, err
	}
	p, err := io.ReadAll(&countingReader{r, m, d})
	return kind, p, err
}

type countingReader struct {
	io.Reader
	meter     *Meter
	direction int
}

func (r *countingReader) Read(p []byte) (int, error) {
	n, e := r.Reader.Read(p)
	r.meter.Add(r.direction, n)
	return n, e
}

type countingWriter struct {
	io.Writer
	meter     *Meter
	direction int
}

func (w *countingWriter) Write(p []byte) (int, error) {
	n, e := w.Writer.Write(p)
	w.meter.Add(w.direction, n)
	return n, e
}
func WriteWS(ctx context.Context, c *coderws.Conn, m *Meter, d int, kind coderws.MessageType, p []byte) error {
	w, err := c.Writer(ctx, kind)
	if err != nil {
		return err
	}
	_, err = (&countingWriter{w, m, d}).Write(p)
	if err != nil {
		_ = w.Close()
		return err
	}
	return w.Close()
}
func WriteWSJSON(ctx context.Context, c *coderws.Conn, m *Meter, d int, value any) error {
	// Preserve wsjson.Write's trailing newline.
	p, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return WriteWS(ctx, c, m, d, coderws.MessageText, append(p, '\n'))
}
