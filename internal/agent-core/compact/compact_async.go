package compact

import (
	"context"
	"sync"
)

type Compactor struct {
	compactFn func(context.Context) error

	once      sync.Once
	done      chan struct{}
	launchErr error
	runErr    error
}

func NewCompactor(compactFn func(context.Context) error) *Compactor {
	return &Compactor{
		compactFn: compactFn,
		done:      make(chan struct{}),
	}
}

func (c *Compactor) CompactAsync(ctx context.Context) error {
	if c == nil || c.compactFn == nil {
		return nil
	}
	c.once.Do(func() {
		ctx2 := ctx
		if ctx2 == nil {
			ctx2 = context.Background()
		}
		c.launchErr = ctx2.Err()
		go func() {
			defer close(c.done)
			c.runErr = c.compactFn(ctx2)
		}()
	})
	return nil
}

func (c *Compactor) WaitIdle(ctx context.Context) error {
	if c == nil {
		return nil
	}
	select {
	case <-c.done:
		return c.runErr
	case <-ctx.Done():
		return ctx.Err()
	}
}
