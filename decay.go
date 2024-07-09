package main

import (
	"math"
	"time"
)

type ForwardDecay struct {
	landmark time.Time
	g        func(time.Duration) float64
}

type Item interface {
	Timestamp() time.Time
	Value() float64
}

type BasicItem struct {
	timestamp time.Time
	value     float64
}

type G func(duration time.Duration) float64

func NewBasicItem(timestamp time.Time, value float64) BasicItem {
	return BasicItem{
		timestamp: timestamp,
		value:     value,
	}
}

func (t BasicItem) Timestamp() time.Time {
	return t.timestamp
}

func (t BasicItem) Value() float64 {
	return t.value
}

func ExponentialDecayFunction(target float64, interval time.Duration) G {
	alpha := math.Log(target) / interval.Seconds()
	return func(duration time.Duration) float64 {
		return math.Exp(alpha * duration.Seconds())
	}
}

func PolynomialDecayFunction(beta float64) G {
	return func(duration time.Duration) float64 {
		return math.Pow(duration.Seconds(), beta)
	}
}

func NewDecay(now time.Time, g G) ForwardDecay {
	return ForwardDecay{
		landmark: now,
		g:        g,
	}
}

func (d ForwardDecay) Landmark() time.Time {
	return d.landmark
}

func (d *ForwardDecay) SetLandmark(landmark time.Time) time.Duration {
	old := d.landmark
	d.landmark = landmark
	return d.landmark.Sub(old)
}

func (d *ForwardDecay) G(duration time.Duration) float64 {
	return d.g(duration)
}

func (d ForwardDecay) Weight(item Item, timestamp time.Time) float64 {
	return d.g(item.Timestamp().Sub(d.landmark)) / d.g(timestamp.Sub(d.landmark))
}

func (d ForwardDecay) WeightedValue(item Item, timestamp time.Time) float64 {
	return d.Weight(item, timestamp) * item.Value()
}

func (d ForwardDecay) StaticWeight(item Item) float64 {
	return d.g(item.Timestamp().Sub(d.landmark))
}

func (d ForwardDecay) StaticWeightedValue(item Item) float64 {
	return d.g(item.Timestamp().Sub(d.landmark)) * item.Value()
}

func (d ForwardDecay) NormalizingFactor(timestamp time.Time) float64 {
	return d.g(timestamp.Sub(d.landmark))
}
