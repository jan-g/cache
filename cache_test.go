package cache

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	"github.com/jan-g/delay"
)

func init() {
	logrus.SetLevel(logrus.DebugLevel)
}

const (
	period = 200 * time.Millisecond
)

var (
	positive = delay.New(2 * period)
	negative = delay.New(period, delay.WithMultiplier(2), delay.WithMaximum(5*period))
)

type refresher struct {
	i         int
	errBefore int
	err       error
	period    time.Duration
}

func (r *refresher) refresh(ctx context.Context, key Key) (Value, error) {
	fmt.Println("refreshing", key, "...")
	if r.i < r.errBefore {
		fmt.Println("refreshed", key, "=", r.err)
		r.i++
		return nil, r.err
	} else {
		select {
		case <-ctx.Done():
			fmt.Println("cancelled refresh of", key)
			return nil, ctx.Err()
		case <-time.After(r.period):
		}
	}
	r.i++
	fmt.Println("refreshed", key, "=", r.i)
	return r.i, nil
}

func TestInitalLoad(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	c := New(ctx, (&refresher{period: period}).refresh, positive, negative)

	v, e := c.Get(context.Background(), "foo")
	assert.Nil(t, e)
	assert.Equal(t, 1, v)

	cancel()
}

func TestRefresh(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := New(ctx, (&refresher{period: period}).refresh, positive, negative)

	v, e := c.Get(context.Background(), "foo")
	assert.Nil(t, e)
	assert.Equal(t, 1, v)

	time.Sleep(period)
	v, e = c.Get(context.Background(), "foo")
	assert.Nil(t, e)
	assert.Equal(t, 1, v)

	time.Sleep(3 * period)
	v, e = c.Get(context.Background(), "foo")
	assert.Nil(t, e)
	assert.Equal(t, 2, v)

	cancel()
}

func TestCleanUnusedValues(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := New(ctx, (&refresher{period: period}).refresh, positive, negative).(*cache)

	v, e := c.Get(context.Background(), "foo")
	assert.Nil(t, e)
	assert.Equal(t, 1, v)

	time.Sleep(period / 2)

	for i := 0; i < 7; i++ {
		// 0.5: still cached
		// 1.5: still cached
		// 2.0: trigger refresh
		// 2.5: refreshing, cached
		// 3.0: refreshed
		// 3.5: cached post-refresh
		// 4.5: cached
		// 5.0 = 3.0 + 2, refresh triggers purge
		// 5.5: uncached
		// 6.5: uncached
		_, ok := c.kv.Load("foo")
		fmt.Println("i=", i, "; key present?", ok)
		assert.Equal(t, i < 5, ok)
		time.Sleep(period)
	}

	// The computation of the next value will be the third
	// time we've called the refresh function
	v, e = c.Get(context.Background(), "foo")
	assert.Nil(t, e)
	assert.Equal(t, 3, v)

	cancel()
}

func TestHardDelay(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := New(ctx, (&refresher{period: 3 * period}).refresh, positive, negative).(*cache)

	v, e := c.Get(context.Background(), "foo")
	assert.Nil(t, e)
	assert.Equal(t, 1, v)

	time.Sleep(period / 2)

	for i := 0; i < 6; i++ {
		// 0.5: still cached
		// 1.5: still cached
		// 2.0: trigger refresh
		// 2.5: refreshing, cached
		// 3.5: refreshing, cached
		// 4.0: second refresh causes warning
		// 4.5: still cached v=1
		// 5.0: refresh finally comes in, v=2
		// 5.5: v=2
		_, ok := c.kv.Load("foo")
		v, e := c.Get(context.Background(), "foo")
		fmt.Println("i=", i, "; key present?", ok, "value=", v)
		assert.Equal(t, 1+(i/5), v)
		assert.Nil(t, e)
		time.Sleep(period)
	}

	// The computation of the next value will be the third
	// time we've called the refresh function
	v, e = c.Get(context.Background(), "foo")
	assert.Nil(t, e)
	assert.Equal(t, 2, v)

	cancel()
}

func TestErrorBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := New(ctx, (&refresher{period: period, errBefore: 2, err: errors.New("an error")}).refresh, positive, negative)

	v, e := c.Get(context.Background(), "foo")
	assert.Equal(t, "an error", e.Error())
	assert.Nil(t, v)

	time.Sleep(period)
	v, e = c.Get(context.Background(), "foo")
	assert.Equal(t, "an error", e.Error())
	assert.Nil(t, v)

	time.Sleep(7 * period)
	v, e = c.Get(context.Background(), "foo")
	assert.Nil(t, e)
	assert.Equal(t, 3, v)

	cancel()
}
