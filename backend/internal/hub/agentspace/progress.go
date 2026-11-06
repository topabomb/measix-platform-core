package agentspace

import (
	"io"
	"sync"
	"time"
)

type progressBody struct {
	body       io.ReadCloser
	mu         sync.Mutex
	timer      *time.Timer
	idle       time.Duration
	closed     bool
	generation uint64
}

func newProgressBody(body io.ReadCloser, idle time.Duration) *progressBody {
	p := &progressBody{body: body, idle: idle}
	p.progress()
	return p
}
func (p *progressBody) progress() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.generation++
	generation := p.generation
	if p.timer != nil {
		p.timer.Stop()
	}
	p.timer = time.AfterFunc(p.idle, func() {
		p.mu.Lock()
		if p.closed || generation != p.generation {
			p.mu.Unlock()
			return
		}
		p.closed = true
		p.mu.Unlock()
		p.body.Close()
	})
}
func (p *progressBody) Read(b []byte) (int, error) {
	n, e := p.body.Read(b)
	if n > 0 {
		p.progress()
	}
	return n, e
}
func (p *progressBody) Close() error {
	p.mu.Lock()
	p.closed = true
	if p.timer != nil {
		p.timer.Stop()
	}
	p.mu.Unlock()
	return p.body.Close()
}
