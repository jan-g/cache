package cache

import (
	"math"
	"math/rand"
	"time"

	"github.com/sirupsen/logrus"
)

type Delay interface {
	Delay() <-chan time.Time
	Reset()
}

type delay struct {
	base   time.Duration
	curr   time.Duration
	mult   float64
	max    time.Duration
	jitter float64
}

func NewDelay(base time.Duration, opts ...DelayOpt) Delay {
	d := &delay{
		base:   base,
		curr:   base,
		mult:   1,
		max:    base,
		jitter: 0,
	}
	for _, o := range opts {
		if err := o(d); err != nil {
			panic(err)
		}
	}
	return d
}

type DelayOpt func(*delay) error

func WithMultiplier(m float64) DelayOpt {
	return func(d *delay) error {
		d.mult = m
		return nil
	}
}

func WithMaximum(max time.Duration) DelayOpt {
	return func(d *delay) error {
		d.max = max
		return nil
	}
}

func WithJitter(j float64) DelayOpt {
	return func(d *delay) error {
		d.jitter = j
		return nil
	}
}

func (d *delay) Delay() <-chan time.Time {
	wait := time.Duration(float64(d.curr) * (1 + rand.Float64()*d.jitter))
	d.curr = time.Duration(math.Min(float64(d.curr)*d.mult, float64(d.max)))
	logrus.WithField("wait", wait).Debug("returning delay")
	return time.After(wait)
}

func (d *delay) Reset() {
	d.curr = d.base
}
