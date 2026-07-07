// Copyright (c) the go-ruby-factory-bot/factory-bot authors
//
// SPDX-License-Identifier: BSD-3-Clause

// Package factorybot is a pure-Go (CGO-free) reimplementation of the
// deterministic core of Ruby's factory_bot gem — the fixtures-replacement
// library used to define factories and construct objects for tests and
// seeding. It reproduces the factory registry, attribute resolution,
// sequences, traits, associations, transient attributes, parent/child
// inheritance, nested factories, and the build/create callback pipeline —
// without any Ruby runtime.
//
// It is the factory_bot engine for
// [go-embedded-ruby](https://github.com/go-embedded-ruby/ruby), but a
// standalone, reusable module.
//
// # What it is — and isn't
//
// Everything factory_bot does to resolve a factory into a bag of attributes is
// deterministic and needs no interpreter, so it lives here as pure Go:
// merging parent attributes, overlaying traits, evaluating static and dynamic
// (lazy) attribute values, advancing sequences, resolving associations,
// separating transient attributes, and driving the callback pipeline in
// factory_bot's order (after(:build) → before(:create) → persist →
// after(:create)).
//
// The two things factory_bot delegates to the object model — instantiating the
// class and persisting it — are host seams here:
//
//   - [BuildFunc] instantiates a class from a resolved, non-transient attribute
//     map and returns the object (factory_bot's initialize_with / attribute
//     assignment). The default seam returns the attribute map itself, so the
//     library is fully exercised without any ORM.
//   - [PersistFunc] saves a built object (factory_bot's to_create, default
//     save!). The default seam is a no-op.
//
// A dynamic attribute value is a Go block seam, [Block]
// (func(*Evaluator) any), mirroring factory_bot's `attr { ... }`. A future
// rbgo binding wires [BuildFunc]/[PersistFunc]/[Block] to Ruby object
// construction, ActiveRecord persistence, and Ruby blocks respectively.
package factorybot
