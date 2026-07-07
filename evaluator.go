// Copyright (c) the go-ruby-factory-bot/factory-bot authors
//
// SPDX-License-Identifier: BSD-3-Clause

package factorybot

import "fmt"

// strategy is the build strategy in effect: build (no persistence) or create
// (persistence). Associations inherit their parent's strategy, matching
// factory_bot's use_parent_strategy default.
type strategy int

const (
	strategyBuild strategy = iota
	strategyCreate
)

// buildContext carries the state threaded through a build: the owning registry,
// the active strategy, and the chain of factory names currently under
// construction (used to detect association cycles).
type buildContext struct {
	reg      *Registry
	strategy strategy
	chain    []string
}

// Evaluator resolves and memoizes a factory's attribute values during a build,
// mirroring factory_bot's evaluator. Dynamic attribute blocks and callbacks
// receive it and read sibling/transient attributes through [Evaluator.Get].
type Evaluator struct {
	attrs map[string]*attribute
	cache map[string]any
	ctx   buildContext
}

// evalPanic wraps an error raised while a dynamic block reads a sibling
// attribute through [Evaluator.Get], so it can surface as a normal error at the
// top of the build rather than a panic.
type evalPanic struct{ err error }

// Get returns the (memoized) value of the named attribute for use inside a
// dynamic block or callback, mirroring reading a sibling in factory_bot's
// evaluator. An unknown name yields nil. If resolving the attribute fails (for
// example a sequence overflow or an association cycle), Get panics with an
// internal marker that the build recovers and returns as an error.
func (e *Evaluator) Get(name string) any {
	v, err := e.getValue(name)
	if err != nil {
		panic(evalPanic{err})
	}
	return v
}

// getValue resolves and memoizes one attribute, returning any error.
func (e *Evaluator) getValue(name string) (result any, err error) {
	if v, ok := e.cache[name]; ok {
		return v, nil
	}
	a := e.attrs[name]
	if a == nil {
		return nil, nil
	}
	switch a.kind {
	case kindStatic:
		result = a.static
	case kindDynamic:
		result, err = e.callBlock(a.dynamic)
		if err != nil {
			return nil, err
		}
	case kindSequence:
		result, err = a.seq.next()
		if err != nil {
			return nil, err
		}
	default: // kindAssociation
		result, err = e.buildAssociation(a)
		if err != nil {
			return nil, err
		}
	}
	e.cache[name] = result
	return result, nil
}

// callBlock runs a dynamic block, converting a nested [Evaluator.Get] failure
// (delivered as an evalPanic) into a returned error.
func (e *Evaluator) callBlock(block Block) (result any, err error) {
	defer func() {
		if r := recover(); r != nil {
			if ep, ok := r.(evalPanic); ok {
				err = ep.err
				return
			}
			panic(r)
		}
	}()
	return block(e), nil
}

// buildAssociation resolves an association attribute by building the referenced
// factory with the parent strategy, guarding against cycles.
func (e *Evaluator) buildAssociation(a *attribute) (any, error) {
	for _, n := range e.ctx.chain {
		if n == a.assoc {
			return nil, fmt.Errorf("%w: %s", ErrAssociationCycle, a.assoc)
		}
	}
	return e.ctx.reg.buildInternal(a.assoc, e.ctx.strategy, a.over, a.traits, e.ctx.chain)
}
