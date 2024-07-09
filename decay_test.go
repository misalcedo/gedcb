package main

import (
	"reflect"
	"testing"
	"time"
)

func NewTestItem(landmark time.Time, offset time.Duration, value float64) BasicItem {
	return NewBasicItem(landmark.Add(time.Second*offset), value)
}

func TestNewDecay(t *testing.T) {
	landmark := time.Now()
	timestamp := landmark.Add(time.Second * 10)
	decay := NewDecay(landmark, PolynomialDecayFunction(2))
	stream := []BasicItem{
		NewTestItem(landmark, 5, 4.0),
		NewTestItem(landmark, 7, 8.0),
		NewTestItem(landmark, 3, 3.0),
		NewTestItem(landmark, 8, 6.0),
		NewTestItem(landmark, 4, 4.0),
	}
	actual := make([]float64, 0, len(stream))

	for _, item := range stream {
		actual = append(actual, decay.Weight(item, timestamp))
	}

	expected := []float64{0.25, 0.49, 0.09, 0.64, 0.16}

	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("got %v\nexpected %v", actual, expected)
	}
}
