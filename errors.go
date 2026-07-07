// Copyright (c) the go-ruby-factory-bot/factory-bot authors
//
// SPDX-License-Identifier: BSD-3-Clause

package factorybot

import "errors"

// Sentinel errors returned by the registry. Match them with [errors.Is].
//
// They mirror the exceptions factory_bot raises: a missing factory or trait
// (FactoryBot::Errors), a sequence that can no longer advance, a duplicate
// definition (DuplicateDefinitionError), a parent chain that loops, and an
// association graph that loops (which factory_bot would otherwise blow the
// stack on).
var (
	// ErrUnknownFactory is returned when a factory name is not registered.
	ErrUnknownFactory = errors.New("factorybot: unknown factory")

	// ErrUnknownTrait is returned when a requested trait is not defined on
	// the factory (or any of its ancestors).
	ErrUnknownTrait = errors.New("factorybot: unknown trait")

	// ErrUnknownSequence is returned by [Registry.Generate] for a global
	// sequence that was never registered.
	ErrUnknownSequence = errors.New("factorybot: unknown sequence")

	// ErrSequenceOverflow is returned when a sequence counter can no longer
	// be advanced without overflowing int64.
	ErrSequenceOverflow = errors.New("factorybot: sequence overflow")

	// ErrDuplicateFactory is returned by [Registry.Define] when a factory
	// with the same name is already registered.
	ErrDuplicateFactory = errors.New("factorybot: duplicate factory")

	// ErrParentCycle is returned when resolving a factory whose parent chain
	// forms a cycle.
	ErrParentCycle = errors.New("factorybot: parent cycle")

	// ErrAssociationCycle is returned when building a factory whose
	// association graph forms a cycle.
	ErrAssociationCycle = errors.New("factorybot: association cycle")
)
