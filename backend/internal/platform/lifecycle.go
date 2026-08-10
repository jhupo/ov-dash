package platform

import "sync"

type Lifecycle struct {
	done chan struct{}
	once sync.Once
}

func NewLifecycle() *Lifecycle {
	return &Lifecycle{done: make(chan struct{})}
}

func (l *Lifecycle) RequestShutdown() {
	if l == nil {
		return
	}
	l.once.Do(func() { close(l.done) })
}

func (l *Lifecycle) Done() <-chan struct{} {
	if l == nil {
		return nil
	}
	return l.done
}
