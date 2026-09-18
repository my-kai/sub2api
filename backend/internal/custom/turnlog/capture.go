package turnlog

import (
	"io"
	"sync"
)

// captureBody preserves the upstream stream while retaining only the configured diagnostic prefix.
type captureBody struct {
	io.ReadCloser
	accountID        int64
	statusCode       int
	headers          map[string][]string
	headersTruncated bool
	buf              []byte
	bodyTruncated    bool
	readDone         bool
	once             sync.Once
	enqueue          func(queuedEvent)
}

func (b *captureBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		remaining := int(MaxBodyBytes) - len(b.buf)
		if remaining > 0 {
			if n < remaining {
				remaining = n
			}
			b.buf = append(b.buf, p[:remaining]...)
		}
		if n > remaining {
			b.bodyTruncated = true
		}
	}
	if err == io.EOF {
		b.readDone = true
		b.finish()
	}
	return n, err
}

func (b *captureBody) Close() error {
	err := b.ReadCloser.Close()
	b.finish()
	return err
}

func (b *captureBody) finish() {
	b.once.Do(func() {
		b.enqueue(queuedEvent{
			AccountID:        b.accountID,
			StatusCode:       b.statusCode,
			Headers:          b.headers,
			HeadersTruncated: b.headersTruncated,
			ResponseBody:     string(b.buf),
			BodyTruncated:    b.bodyTruncated,
			BodyComplete:     b.readDone,
		})
	})
}

var _ io.ReadCloser = (*captureBody)(nil)
