// Copyright (c) the go-ruby-factory-bot/factory-bot authors
//
// SPDX-License-Identifier: BSD-3-Clause

package factorybot

// Block is the seam for a dynamic (lazy) attribute value, mirroring
// factory_bot's `name { ... }`. It is evaluated at build time and may read
// sibling attributes through the [Evaluator] it is handed (which memoizes each
// value, exactly like factory_bot's evaluator).
type Block func(e *Evaluator) any

// Callback is a build/create lifecycle hook, mirroring factory_bot's
// after(:build) / before(:create) / after(:create). It receives the object
// produced by the [BuildFunc] seam and the [Evaluator], so it can read
// transient attributes. Returning a non-nil error aborts the build.
type Callback func(obj any, e *Evaluator) error

// attrKind enumerates the four flavours of a factory attribute.
type attrKind int

const (
	kindStatic attrKind = iota
	kindDynamic
	kindSequence
	kindAssociation
)

// attribute is one declared attribute of a factory (or trait).
type attribute struct {
	name      string
	kind      attrKind
	transient bool

	static  any            // kindStatic
	dynamic Block          // kindDynamic
	seq     *Sequence      // kindSequence
	assoc   string         // kindAssociation: target factory name
	over    map[string]any // kindAssociation: attribute overrides
	traits  []string       // kindAssociation: traits to apply
}

// attrList is an insertion-ordered set of attributes keyed by name. Re-setting
// an existing name replaces its definition in place (later wins), matching how
// factory_bot lets child factories, traits, and runtime overrides shadow an
// attribute without changing its position.
type attrList struct {
	order  []string
	byName map[string]*attribute
}

func newAttrList() *attrList {
	return &attrList{byName: map[string]*attribute{}}
}

func (l *attrList) set(a *attribute) {
	if _, ok := l.byName[a.name]; !ok {
		l.order = append(l.order, a.name)
	}
	l.byName[a.name] = a
}

func (l *attrList) list() []*attribute {
	out := make([]*attribute, 0, len(l.order))
	for _, n := range l.order {
		out = append(out, l.byName[n])
	}
	return out
}

// Definition is the builder handed to a define/factory/trait/transient block.
// It records a factory's class, parent, attributes, sequences, associations,
// traits, nested factories, and callbacks. The same type backs traits and
// transient blocks (which only use a subset of its methods).
type Definition struct {
	name   string
	class  string
	parent string
	attrs  *attrList
	traits map[string]*Definition
	nested []*Definition

	afterBuild   []Callback
	beforeCreate []Callback
	afterCreate  []Callback
}

func newDefinition(name string) *Definition {
	return &Definition{
		name:   name,
		attrs:  newAttrList(),
		traits: map[string]*Definition{},
	}
}

// Class sets the class name passed to the [BuildFunc] seam, mirroring
// `factory :user, class: "User"`. When unset it defaults to the factory name
// (or is inherited from the parent).
func (d *Definition) Class(class string) *Definition {
	d.class = class
	return d
}

// Parent sets the parent factory to inherit attributes, traits, and callbacks
// from, mirroring `factory :admin, parent: :user`.
func (d *Definition) Parent(parent string) *Definition {
	d.parent = parent
	return d
}

// Attr declares a static attribute, mirroring `name "value"`.
func (d *Definition) Attr(name string, value any) *Definition {
	d.attrs.set(&attribute{name: name, kind: kindStatic, static: value})
	return d
}

// Dynamic declares a lazily evaluated attribute, mirroring `name { ... }`. The
// block may read sibling attributes through the [Evaluator].
func (d *Definition) Dynamic(name string, block Block) *Definition {
	d.attrs.set(&attribute{name: name, kind: kindDynamic, dynamic: block})
	return d
}

// Sequence declares a per-factory sequence starting at 1, mirroring
// `sequence(:email) { |n| ... }`. Pass a nil generator to yield the raw number.
func (d *Definition) Sequence(name string, gen func(n int) any) *Definition {
	return d.SequenceFrom(name, 1, gen)
}

// SequenceFrom is [Definition.Sequence] with an explicit starting value,
// mirroring `sequence(:email, 1000) { |n| ... }`.
func (d *Definition) SequenceFrom(name string, start int64, gen func(n int) any) *Definition {
	d.attrs.set(&attribute{name: name, kind: kindSequence, seq: newSequence(start, gen)})
	return d
}

// Association declares an association to another factory, mirroring
// `association :author, factory: :user, name: "x"`. An empty factoryName
// defaults to name. The association is built with the parent strategy (build
// within build, create within create). overrides and traits are applied to the
// associated build.
func (d *Definition) Association(name, factoryName string, overrides map[string]any, traits ...string) *Definition {
	if factoryName == "" {
		factoryName = name
	}
	d.attrs.set(&attribute{
		name:   name,
		kind:   kindAssociation,
		assoc:  factoryName,
		over:   overrides,
		traits: traits,
	})
	return d
}

// Transient declares transient attributes, mirroring `transient do ... end`.
// They are available to other attributes and callbacks through the evaluator
// but are never passed to the [BuildFunc] seam.
func (d *Definition) Transient(fn func(t *Definition)) *Definition {
	sub := newDefinition("")
	fn(sub)
	for _, a := range sub.attrs.list() {
		a.transient = true
		d.attrs.set(a)
	}
	return d
}

// Trait declares a named attribute overlay, mirroring `trait :admin do ... end`.
// A trait may itself declare attributes and callbacks; applying it overlays
// them at build time.
func (d *Definition) Trait(name string, fn func(t *Definition)) *Definition {
	sub := newDefinition(name)
	fn(sub)
	d.traits[name] = sub
	return d
}

// Factory declares a nested (child) factory that inherits from the enclosing
// one, mirroring a `factory :admin do ... end` nested inside another factory.
func (d *Definition) Factory(name string, fn func(f *Definition)) *Definition {
	sub := newDefinition(name)
	fn(sub)
	d.nested = append(d.nested, sub)
	return d
}

// AfterBuild registers an after(:build) callback.
func (d *Definition) AfterBuild(cb Callback) *Definition {
	d.afterBuild = append(d.afterBuild, cb)
	return d
}

// BeforeCreate registers a before(:create) callback.
func (d *Definition) BeforeCreate(cb Callback) *Definition {
	d.beforeCreate = append(d.beforeCreate, cb)
	return d
}

// AfterCreate registers an after(:create) callback.
func (d *Definition) AfterCreate(cb Callback) *Definition {
	d.afterCreate = append(d.afterCreate, cb)
	return d
}
