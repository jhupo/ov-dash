package events

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Publisher interface {
	Publish(context.Context, Event) error
}

type Dispatcher struct {
	outbox    *Outbox
	publisher Publisher
	owner     string
	batchSize int
	lease     time.Duration
}

type DispatchResult struct {
	Claimed int
	Acked   int
	Failed  int
}

func NewDispatcher(outbox *Outbox, publisher Publisher, owner string) (*Dispatcher, error) {
	owner = strings.TrimSpace(owner)
	if outbox == nil {
		return nil, errors.New("create outbox dispatcher: outbox is nil")
	}
	if publisher == nil {
		return nil, errors.New("create outbox dispatcher: publisher is nil")
	}
	if owner == "" {
		return nil, errors.New("create outbox dispatcher: owner is required")
	}
	return &Dispatcher{
		outbox:    outbox,
		publisher: publisher,
		owner:     owner,
		batchSize: 100,
		lease:     30 * time.Second,
	}, nil
}

func (d *Dispatcher) DispatchBatch(ctx context.Context) (DispatchResult, error) {
	records, err := d.outbox.Claim(ctx, d.owner, d.batchSize, d.lease)
	if err != nil {
		return DispatchResult{}, err
	}
	result := DispatchResult{Claimed: len(records)}
	var dispatchErrors []error
	for _, record := range records {
		if err := d.publisher.Publish(ctx, record.Event); err != nil {
			result.Failed++
			if failErr := d.outbox.Fail(ctx, record, d.owner, err); failErr != nil {
				dispatchErrors = append(dispatchErrors, fmt.Errorf("release failed event %s: %w", record.ID, failErr))
			}
			continue
		}
		if err := d.outbox.Ack(ctx, record.ID, d.owner); err != nil {
			dispatchErrors = append(dispatchErrors, fmt.Errorf("ack event %s: %w", record.ID, err))
			continue
		}
		result.Acked++
	}
	return result, errors.Join(dispatchErrors...)
}
