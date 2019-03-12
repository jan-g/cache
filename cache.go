package cache

import (
	"context"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/jan-g/delay"
)

type Key interface{}
type Value interface{}
type Refresher func(ctx context.Context, key Key) (value Value, err error)

type Cache interface {
	Get(context.Context, Key) (Value, error)
}

type cache struct {
	ctx       context.Context
	refresher Refresher
	positive  delay.Delay
	negative  delay.Delay
	kv        sync.Map // Key: <-chan r
}

// Package up a result, error pair.
type r struct {
	Value
	Err error
}

func New(ctx context.Context, refresher Refresher, positive delay.Delay, negative delay.Delay) Cache {
	return &cache{
		ctx:       ctx,
		refresher: refresher,
		positive:  positive,
		negative:  negative,
	}
}

func (cache *cache) Get(ctx context.Context, key Key) (Value, error) {
	for {
		newCh := make(chan r)
		c, loaded := cache.kv.LoadOrStore(key, newCh)
		ch := c.(chan r)
		if !loaded {
			go cache.maintain(cache.ctx, key, ch)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case result, ok := <-ch:
			if ok {
				return result.Value, result.Err
			}
			// The channel was closed; we need to update the store with a new maintainer
			// If two Get calls race here, one will come out the victor; the other maintenance
			// loop will time out after a refresh
			cache.kv.Delete(key)
			continue
		}
	}
}

func (cache *cache) maintain(ctx context.Context, key Key, ch chan<- r) {
	log := logrus.WithField("key", key)

	refreshCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Initialise the refresh loop
	refresh := make(chan r, 1)
	refreshing := false
	var nextRefresh <-chan time.Time

	// Generate the initial value
	value, err := cache.refresher(ctx, key)
	log.WithField("value", value).WithError(err).Debug("initialised value")
	if err == nil {
		cache.positive.Reset()
		cache.negative.Reset()
		nextRefresh = cache.positive.Delay()
	} else {
		nextRefresh = cache.negative.Delay()
	}
	result := r{Value: value, Err: err}

	// Keep tabs on whether this value has been recently referred to
	used := false
loop:
	for {
		select {
		case <-ctx.Done():
			log.Debug("maintenance loop exits")
			break loop
		case ch <- result:
			// We just send the updated r
			used = true
			log.WithField("value", result.Value).WithError(result.Err).Debug("value returned")
		case <-nextRefresh:
			if !used {
				// We've not been requested for an entire refresh positive
				log.Debug("refresh on unused value, exiting")
				break loop
			}
			used = false
			// We may already be refreshing; don't do it twice
			if !refreshing {
				refreshing = true
				log.Debug("triggering a refresh")
				go cache.refresh(refreshCtx, key, refresh)
				goto timer_reset
			} else {
				// If we've waited twice the refresh amount, warn
				log.Warn("second time refreshing without value")
			}
		case result = <-refresh:
			goto refresh
		}
		continue loop

	refresh:
		refreshing = false
		log.WithField("value", result.Value).WithError(result.Err).Debug("refreshed value")
	timer_reset:
		if result.Err == nil {
			cache.positive.Reset()
			cache.negative.Reset()
			nextRefresh = cache.positive.Delay()
		} else {
			nextRefresh = cache.negative.Delay()
		}
	}

	cache.kv.Delete(key)
	close(ch)
}

func (cache *cache) refresh(ctx context.Context, key Key, refresh chan<- r) {
	value, err := cache.refresher(ctx, key)
	refresh <- r{Value: value, Err: err}
}
