package servers

import "context"

type CollectionEvent struct {
	ServerID string
	Stage    string
	Mode     string
	Stream   string
	Message  string
	Metadata map[string]any
}

type CollectionObserver interface {
	ObserveCollection(ctx context.Context, event CollectionEvent)
}

type CollectionObserverFunc func(ctx context.Context, event CollectionEvent)

func (f CollectionObserverFunc) ObserveCollection(ctx context.Context, event CollectionEvent) {
	if f != nil {
		f(ctx, event)
	}
}
