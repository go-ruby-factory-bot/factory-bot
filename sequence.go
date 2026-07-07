// Copyright (c) the go-ruby-factory-bot/factory-bot authors
//
// SPDX-License-Identifier: BSD-3-Clause

package factorybot

import "math"

// Sequence is a monotonically increasing counter with an optional generator,
// mirroring factory_bot's sequence(:name) { |n| ... }. The counter starts at 1
// by default (factory_bot's default) and is shared across every build that
// touches it, so successive objects receive successive values.
type Sequence struct {
	counter int64
	gen     func(n int) any
}

// newSequence creates a sequence starting at start with generator gen (gen may
// be nil, in which case the raw counter value is returned).
func newSequence(start int64, gen func(n int) any) *Sequence {
	return &Sequence{counter: start, gen: gen}
}

// next returns the sequence's current value and advances the counter. It
// returns [ErrSequenceOverflow] once the counter has reached math.MaxInt64 and
// can no longer advance.
func (s *Sequence) next() (any, error) {
	if s.counter == math.MaxInt64 {
		return nil, ErrSequenceOverflow
	}
	n := s.counter
	s.counter++
	if s.gen != nil {
		return s.gen(int(n)), nil
	}
	return n, nil
}
