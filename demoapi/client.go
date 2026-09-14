package demoapi

import (
	"io"
	"net/http"
)

// Client returns an *http.Client that dispatches straight into h.
//
// No port, no bind, no cleanup, and no flake when twenty examples start at
// once. A screen still builds an *http.Request and still parses JSON out of an
// *http.Response, so nothing about how it is written changes — which is the
// point: `task examples` needs no sidecar running, and the same screen works
// unaltered against a real server.
//
// Streaming works. See inProcess.RoundTrip for why that took a pipe.
func Client(h http.Handler) *http.Client {
	return &http.Client{Transport: inProcess{h: h}}
}

type inProcess struct{ h http.Handler }

// RoundTrip runs the handler against a pipe rather than a buffer.
//
// httptest.NewRecorder is the obvious choice and is wrong here: it collects
// the whole response and hands it back when the handler returns. For the list
// endpoints that is invisible, and for the job log it is silently misleading —
// a stream that never ends would hang, and one that does would arrive in a
// single read, so a screen consuming it would look correct while testing
// nothing about partial arrival.
//
// So: a pipe, a goroutine running ServeHTTP, and a response whose Body is the
// read half. The handler's Flush lands on the pipe, and the client sees lines
// as they are written.
func (t inProcess) RoundTrip(r *http.Request) (*http.Response, error) {
	pr, pw := io.Pipe()
	rw := &pipeWriter{w: pw, header: http.Header{}, done: make(chan struct{})}

	go func() {
		defer pw.Close()
		t.h.ServeHTTP(rw, r.WithContext(r.Context()))
		rw.finish()
	}()

	// Wait for the status line before returning, so a caller that checks
	// StatusCode is not racing the handler.
	select {
	case <-rw.done:
	case <-r.Context().Done():
		pr.CloseWithError(r.Context().Err())
		return nil, r.Context().Err()
	}

	return &http.Response{
		Status:     http.StatusText(rw.code),
		StatusCode: rw.code,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     rw.header,
		Body:       pr,
		Request:    r,
	}, nil
}

// pipeWriter is the http.ResponseWriter half of the pipe. Flush is a no-op
// because a pipe has no buffer to flush — a write is already a read on the
// other side, which is exactly the property the recorder lacks.
type pipeWriter struct {
	w      *io.PipeWriter
	header http.Header
	code   int
	done   chan struct{}
	closed bool
}

func (p *pipeWriter) Header() http.Header { return p.header }

func (p *pipeWriter) WriteHeader(code int) {
	if p.closed {
		return
	}
	p.code = code
	p.closed = true
	close(p.done)
}

func (p *pipeWriter) Write(b []byte) (int, error) {
	if !p.closed {
		p.WriteHeader(http.StatusOK)
	}
	return p.w.Write(b)
}

func (p *pipeWriter) Flush() {}

// finish releases a caller waiting on a handler that returned without ever
// writing anything.
func (p *pipeWriter) finish() {
	if !p.closed {
		p.WriteHeader(http.StatusOK)
	}
}
