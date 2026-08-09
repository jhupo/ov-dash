package events

import (
	"context"
	"errors"
	"sync"

	"go.uber.org/zap"
)

type Handler func(context.Context, Event) error

type Bus struct {
	logger       *zap.Logger
	mu           sync.RWMutex
	subscribers  map[string][]Handler
	allHandlers  []Handler
	errorHandler func(context.Context, Event, error)
}

func NewBus(logger *zap.Logger) *Bus {
	return &Bus{
		logger:      logger,
		subscribers: map[string][]Handler{},
		errorHandler: func(ctx context.Context, event Event, err error) {
			logger.Error("event handler failed",
				zap.String("event_id", event.ID),
				zap.String("event_type", event.Type),
				zap.Error(err),
			)
		},
	}
}

func (b *Bus) Subscribe(eventType string, handler Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subscribers[eventType] = append(b.subscribers[eventType], handler)
}

func (b *Bus) SubscribeAll(handler Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.allHandlers = append(b.allHandlers, handler)
}

func (b *Bus) Publish(ctx context.Context, event Event) error {
	handlers := b.handlers(event.Type)
	var publishErrors []error
	for _, handler := range handlers {
		if err := handler(ctx, event); err != nil {
			publishErrors = append(publishErrors, err)
			if b.errorHandler != nil {
				b.errorHandler(ctx, event, err)
			}
		}
	}
	return errors.Join(publishErrors...)
}

func (b *Bus) handlers(eventType string) []Handler {
	b.mu.RLock()
	defer b.mu.RUnlock()

	handlers := make([]Handler, 0, len(b.allHandlers)+len(b.subscribers[eventType]))
	handlers = append(handlers, b.allHandlers...)
	handlers = append(handlers, b.subscribers[eventType]...)
	return handlers
}

func LogHandler(logger *zap.Logger) Handler {
	return func(_ context.Context, event Event) error {
		logger.Info("event published",
			zap.String("event_id", event.ID),
			zap.String("event_type", event.Type),
			zap.String("source", event.Source),
			zap.Any("payload", event.Payload),
		)
		return nil
	}
}
