// Copyright (c) the go-ruby-factory-bot/factory-bot authors
//
// SPDX-License-Identifier: BSD-3-Clause

package factorybot

import "fmt"

// BuildFunc is the object-instantiation seam, mirroring factory_bot's
// initialize_with plus attribute assignment. It receives the resolved class
// name and the non-transient attribute map and returns the constructed object.
// The default seam returns a copy of the attribute map, so the engine runs
// without any object model.
type BuildFunc func(class string, attrs map[string]any) (any, error)

// PersistFunc is the persistence seam, mirroring factory_bot's to_create
// (default save!). It is invoked by Create/CreateList after the object is
// built and before(:create) callbacks have run. The default seam is a no-op.
type PersistFunc func(class string, obj any) error

// Registry holds factory definitions and global sequences and constructs
// objects, mirroring FactoryBot's module-level registry.
type Registry struct {
	factories map[string]*Definition
	globalSeq map[string]*Sequence
	buildFn   BuildFunc
	persistFn PersistFunc
}

// New returns an empty registry with default (map-returning / no-op) seams.
func New() *Registry {
	return &Registry{
		factories: map[string]*Definition{},
		globalSeq: map[string]*Sequence{},
		buildFn:   defaultBuild,
		persistFn: defaultPersist,
	}
}

func defaultBuild(_ string, attrs map[string]any) (any, error) {
	out := make(map[string]any, len(attrs))
	for k, v := range attrs {
		out[k] = v
	}
	return out, nil
}

func defaultPersist(_ string, _ any) error { return nil }

// SetBuild installs the object-instantiation seam (see [BuildFunc]).
func (r *Registry) SetBuild(fn BuildFunc) { r.buildFn = fn }

// SetPersist installs the persistence seam (see [PersistFunc]).
func (r *Registry) SetPersist(fn PersistFunc) { r.persistFn = fn }

// Define registers a factory, mirroring `factory :name do ... end` inside
// `FactoryBot.define`. The block configures the factory through its
// [Definition]; nested factories are registered as children. It returns
// [ErrDuplicateFactory] if the name (or any nested name) is already registered.
func (r *Registry) Define(name string, fn func(d *Definition)) error {
	d := newDefinition(name)
	fn(d)
	return r.register(d, "")
}

func (r *Registry) register(d *Definition, parent string) error {
	if _, exists := r.factories[d.name]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateFactory, d.name)
	}
	if parent != "" {
		d.parent = parent
	}
	r.factories[d.name] = d
	for _, child := range d.nested {
		if err := r.register(child, d.name); err != nil {
			return err
		}
	}
	return nil
}

// Sequence registers a global sequence starting at 1, mirroring a top-level
// `sequence(:email) { |n| ... }`. Advance it with [Registry.Generate].
func (r *Registry) Sequence(name string, gen func(n int) any) {
	r.SequenceFrom(name, 1, gen)
}

// SequenceFrom is [Registry.Sequence] with an explicit starting value.
func (r *Registry) SequenceFrom(name string, start int64, gen func(n int) any) {
	r.globalSeq[name] = newSequence(start, gen)
}

// Generate advances a global sequence and returns its value, mirroring
// `FactoryBot.generate(:email)`. It returns [ErrUnknownSequence] if no such
// sequence is registered.
func (r *Registry) Generate(name string) (any, error) {
	s, ok := r.globalSeq[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownSequence, name)
	}
	return s.next()
}

// resolved is a fully merged view of a factory: its class, ordered attributes,
// available traits, and callbacks.
type resolved struct {
	class        string
	attrs        *attrList
	traits       map[string]*Definition
	afterBuild   []Callback
	beforeCreate []Callback
	afterCreate  []Callback
}

// resolve merges a factory with its parent chain (guarding against cycles).
func (r *Registry) resolve(name string, seen map[string]bool) (*resolved, error) {
	def, ok := r.factories[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownFactory, name)
	}
	if seen[name] {
		return nil, fmt.Errorf("%w: %s", ErrParentCycle, name)
	}
	seen[name] = true

	var res *resolved
	if def.parent != "" {
		base, err := r.resolve(def.parent, seen)
		if err != nil {
			return nil, err
		}
		res = base
	} else {
		res = &resolved{attrs: newAttrList(), traits: map[string]*Definition{}}
	}

	if def.class != "" {
		res.class = def.class
	}
	if res.class == "" {
		res.class = name
	}
	for _, a := range def.attrs.list() {
		res.attrs.set(a)
	}
	for tn, td := range def.traits {
		res.traits[tn] = td
	}
	res.afterBuild = append(res.afterBuild, def.afterBuild...)
	res.beforeCreate = append(res.beforeCreate, def.beforeCreate...)
	res.afterCreate = append(res.afterCreate, def.afterCreate...)
	return res, nil
}

// resolveWithTraits resolves a factory then overlays the requested traits and
// runtime overrides.
func (r *Registry) resolveWithTraits(name string, traits []string, overrides map[string]any) (*resolved, error) {
	res, err := r.resolve(name, map[string]bool{})
	if err != nil {
		return nil, err
	}
	for _, tn := range traits {
		td, ok := res.traits[tn]
		if !ok {
			return nil, fmt.Errorf("%w: %s (factory %s)", ErrUnknownTrait, tn, name)
		}
		for _, a := range td.attrs.list() {
			res.attrs.set(a)
		}
		res.afterBuild = append(res.afterBuild, td.afterBuild...)
		res.beforeCreate = append(res.beforeCreate, td.beforeCreate...)
		res.afterCreate = append(res.afterCreate, td.afterCreate...)
	}
	for k, v := range overrides {
		transient := false
		if ex, ok := res.attrs.byName[k]; ok && ex.transient {
			transient = true
		}
		res.attrs.set(&attribute{name: k, kind: kindStatic, static: v, transient: transient})
	}
	return res, nil
}

// callOptions accumulates the traits and overrides requested for a build.
type callOptions struct {
	traits    []string
	overrides map[string]any
}

// Opt configures a Build/Create/AttributesFor call.
type Opt func(*callOptions)

// WithTrait requests one or more traits, mirroring `build(:user, :admin)`.
func WithTrait(names ...string) Opt {
	return func(o *callOptions) { o.traits = append(o.traits, names...) }
}

// With overrides a single attribute, mirroring `build(:user, name: "x")`.
func With(name string, value any) Opt {
	return func(o *callOptions) { o.overrides[name] = value }
}

// WithAttrs overrides several attributes at once.
func WithAttrs(m map[string]any) Opt {
	return func(o *callOptions) {
		for k, v := range m {
			o.overrides[k] = v
		}
	}
}

func collect(opts []Opt) *callOptions {
	o := &callOptions{overrides: map[string]any{}}
	for _, fn := range opts {
		fn(o)
	}
	return o
}

// Build constructs an object without persisting it, mirroring
// `FactoryBot.build(:user, ...)`.
func (r *Registry) Build(name string, opts ...Opt) (any, error) {
	o := collect(opts)
	return r.buildInternal(name, strategyBuild, o.overrides, o.traits, nil)
}

// Create constructs and persists an object, mirroring
// `FactoryBot.create(:user, ...)`.
func (r *Registry) Create(name string, opts ...Opt) (any, error) {
	o := collect(opts)
	return r.buildInternal(name, strategyCreate, o.overrides, o.traits, nil)
}

// BuildList builds n objects, mirroring `FactoryBot.build_list(:user, n, ...)`.
func (r *Registry) BuildList(name string, n int, opts ...Opt) ([]any, error) {
	o := collect(opts)
	var out []any
	for i := 0; i < n; i++ {
		obj, err := r.buildInternal(name, strategyBuild, o.overrides, o.traits, nil)
		if err != nil {
			return nil, err
		}
		out = append(out, obj)
	}
	return out, nil
}

// CreateList creates n objects, mirroring `FactoryBot.create_list(:user, n, ...)`.
func (r *Registry) CreateList(name string, n int, opts ...Opt) ([]any, error) {
	o := collect(opts)
	var out []any
	for i := 0; i < n; i++ {
		obj, err := r.buildInternal(name, strategyCreate, o.overrides, o.traits, nil)
		if err != nil {
			return nil, err
		}
		out = append(out, obj)
	}
	return out, nil
}

// AttributesFor returns the resolved non-transient, non-association attributes
// without instantiating anything, mirroring
// `FactoryBot.attributes_for(:user, ...)`.
func (r *Registry) AttributesFor(name string, opts ...Opt) (map[string]any, error) {
	o := collect(opts)
	res, err := r.resolveWithTraits(name, o.traits, o.overrides)
	if err != nil {
		return nil, err
	}
	ev := &Evaluator{
		attrs: res.attrs.byName,
		cache: map[string]any{},
		ctx:   buildContext{reg: r, strategy: strategyBuild, chain: []string{name}},
	}
	out := map[string]any{}
	for _, a := range res.attrs.list() {
		if a.transient || a.kind == kindAssociation {
			continue
		}
		v, err := ev.getValue(a.name)
		if err != nil {
			return nil, err
		}
		out[a.name] = v
	}
	return out, nil
}

// buildInternal is the shared build/create engine. chain is the stack of
// factory names already under construction (for association-cycle detection).
func (r *Registry) buildInternal(name string, strat strategy, overrides map[string]any, traits, chain []string) (any, error) {
	res, err := r.resolveWithTraits(name, traits, overrides)
	if err != nil {
		return nil, err
	}
	ev := &Evaluator{
		attrs: res.attrs.byName,
		cache: map[string]any{},
		ctx: buildContext{
			reg:      r,
			strategy: strat,
			chain:    append(append([]string{}, chain...), name),
		},
	}

	objAttrs := map[string]any{}
	for _, a := range res.attrs.list() {
		if a.transient {
			continue
		}
		v, err := ev.getValue(a.name)
		if err != nil {
			return nil, err
		}
		objAttrs[a.name] = v
	}

	obj, err := r.buildFn(res.class, objAttrs)
	if err != nil {
		return nil, err
	}
	if err := runCallbacks(res.afterBuild, obj, ev); err != nil {
		return nil, err
	}
	if strat == strategyCreate {
		if err := runCallbacks(res.beforeCreate, obj, ev); err != nil {
			return nil, err
		}
		if err := r.persistFn(res.class, obj); err != nil {
			return nil, err
		}
		if err := runCallbacks(res.afterCreate, obj, ev); err != nil {
			return nil, err
		}
	}
	return obj, nil
}

func runCallbacks(cbs []Callback, obj any, e *Evaluator) error {
	for _, cb := range cbs {
		if err := cb(obj, e); err != nil {
			return err
		}
	}
	return nil
}
