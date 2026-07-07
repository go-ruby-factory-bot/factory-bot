// Copyright (c) the go-ruby-factory-bot/factory-bot authors
//
// SPDX-License-Identifier: BSD-3-Clause

package factorybot

import (
	"errors"
	"fmt"
	"math"
	"testing"
)

// asMap unwraps the default BuildFunc's object (a map) for assertions.
func asMap(t *testing.T, obj any) map[string]any {
	t.Helper()
	m, ok := obj.(map[string]any)
	if !ok {
		t.Fatalf("expected map object, got %T", obj)
	}
	return m
}

func mustDefine(t *testing.T, r *Registry, name string, fn func(*Definition)) {
	t.Helper()
	if err := r.Define(name, fn); err != nil {
		t.Fatalf("Define(%q): %v", name, err)
	}
}

// --- basic build: static + dynamic + memoization + Get(missing) -------------

func TestBuildStaticAndDynamic(t *testing.T) {
	r := New()
	calls := 0
	mustDefine(t, r, "user", func(d *Definition) {
		d.Attr("first", "Ada")
		d.Attr("last", "Lovelace")
		d.Dynamic("full", func(e *Evaluator) any {
			calls++
			// read the same sibling twice to prove memoization (one eval).
			return e.Get("first").(string) + " " + e.Get("last").(string) + " " + e.Get("first").(string)
		})
		d.Dynamic("missing", func(e *Evaluator) any {
			return e.Get("nope") // unknown attribute -> nil
		})
	})

	obj, err := r.Build("user")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	m := asMap(t, obj)
	if m["full"] != "Ada Lovelace Ada" {
		t.Fatalf("full = %v", m["full"])
	}
	if m["missing"] != nil {
		t.Fatalf("missing = %v, want nil", m["missing"])
	}
	if calls != 1 {
		t.Fatalf("dynamic evaluated %d times, want 1 (memoized)", calls)
	}
}

// --- class routing through the Build seam ----------------------------------

func TestClassRoutingAndSeam(t *testing.T) {
	r := New()
	var gotClass string
	var gotAttrs map[string]any
	r.SetBuild(func(class string, attrs map[string]any) (any, error) {
		gotClass = class
		gotAttrs = attrs
		return struct{ Tag string }{Tag: class}, nil
	})
	mustDefine(t, r, "user", func(d *Definition) {
		d.Class("User")
		d.Attr("name", "x")
	})
	obj, err := r.Build("user")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if gotClass != "User" {
		t.Fatalf("class = %q", gotClass)
	}
	if gotAttrs["name"] != "x" {
		t.Fatalf("attrs = %v", gotAttrs)
	}
	if obj.(struct{ Tag string }).Tag != "User" {
		t.Fatalf("obj = %v", obj)
	}
}

// default class == factory name when unset.
func TestDefaultClassIsName(t *testing.T) {
	r := New()
	var gotClass string
	r.SetBuild(func(class string, attrs map[string]any) (any, error) {
		gotClass = class
		return attrs, nil
	})
	mustDefine(t, r, "widget", func(d *Definition) { d.Attr("a", 1) })
	if _, err := r.Build("widget"); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if gotClass != "widget" {
		t.Fatalf("default class = %q, want widget", gotClass)
	}
}

// --- sequences: per-factory + global + overflow (direct & via block) --------

func TestPerFactorySequence(t *testing.T) {
	r := New()
	mustDefine(t, r, "user", func(d *Definition) {
		d.Sequence("email", func(n int) any { return fmt.Sprintf("user%d@example.com", n) })
		d.SequenceFrom("code", 10, nil) // nil gen -> raw counter
	})
	o1, _ := r.Build("user")
	o2, _ := r.Build("user")
	m1, m2 := asMap(t, o1), asMap(t, o2)
	if m1["email"] != "user1@example.com" || m2["email"] != "user2@example.com" {
		t.Fatalf("emails: %v %v", m1["email"], m2["email"])
	}
	if m1["code"].(int64) != 10 || m2["code"].(int64) != 11 {
		t.Fatalf("codes: %v %v", m1["code"], m2["code"])
	}
}

func TestGlobalSequence(t *testing.T) {
	r := New()
	r.Sequence("token", func(n int) any { return n * 2 })
	v1, err := r.Generate("token")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	v2, _ := r.Generate("token")
	if v1 != 2 || v2 != 4 {
		t.Fatalf("tokens: %v %v", v1, v2)
	}
}

func TestUnknownSequence(t *testing.T) {
	r := New()
	if _, err := r.Generate("nope"); !errors.Is(err, ErrUnknownSequence) {
		t.Fatalf("err = %v, want ErrUnknownSequence", err)
	}
}

func TestSequenceOverflowDirect(t *testing.T) {
	r := New()
	r.SequenceFrom("of", math.MaxInt64, nil)
	if _, err := r.Generate("of"); !errors.Is(err, ErrSequenceOverflow) {
		t.Fatalf("err = %v, want ErrSequenceOverflow", err)
	}
}

func TestSequenceOverflowInFactoryAttr(t *testing.T) {
	r := New()
	mustDefine(t, r, "user", func(d *Definition) {
		d.SequenceFrom("id", math.MaxInt64, nil)
	})
	if _, err := r.Build("user"); !errors.Is(err, ErrSequenceOverflow) {
		t.Fatalf("err = %v, want ErrSequenceOverflow", err)
	}
}

// overflow surfaced through a dynamic block reading an overflowing sequence.
// The sequence is transient so it is only ever evaluated *through* the block
// (via Evaluator.Get), exercising the error-propagation-out-of-a-block path.
func TestSequenceOverflowInDynamicBlock(t *testing.T) {
	r := New()
	mustDefine(t, r, "user", func(d *Definition) {
		d.Transient(func(tr *Definition) { tr.SequenceFrom("id", math.MaxInt64, nil) })
		d.Dynamic("label", func(e *Evaluator) any {
			return fmt.Sprintf("id-%v", e.Get("id"))
		})
	})
	_, err := r.Build("user")
	if !errors.Is(err, ErrSequenceOverflow) {
		t.Fatalf("err = %v, want ErrSequenceOverflow", err)
	}
}

// a genuine panic inside a dynamic block is not swallowed.
func TestDynamicBlockGenuinePanicPropagates(t *testing.T) {
	r := New()
	mustDefine(t, r, "user", func(d *Definition) {
		d.Dynamic("boom", func(e *Evaluator) any { panic("kaboom") })
	})
	defer func() {
		if rec := recover(); rec == nil {
			t.Fatalf("expected panic to propagate")
		} else if rec != "kaboom" {
			t.Fatalf("unexpected panic value: %v", rec)
		}
	}()
	_, _ = r.Build("user")
}

// --- traits -----------------------------------------------------------------

func TestTraits(t *testing.T) {
	r := New()
	adminAfter := 0
	mustDefine(t, r, "user", func(d *Definition) {
		d.Attr("role", "member")
		d.Trait("admin", func(tr *Definition) {
			tr.Attr("role", "admin")
			tr.AfterBuild(func(obj any, e *Evaluator) error { adminAfter++; return nil })
		})
	})
	base, _ := r.Build("user")
	if asMap(t, base)["role"] != "member" {
		t.Fatalf("base role = %v", asMap(t, base)["role"])
	}
	adm, _ := r.Build("user", WithTrait("admin"))
	if asMap(t, adm)["role"] != "admin" {
		t.Fatalf("admin role = %v", asMap(t, adm)["role"])
	}
	if adminAfter != 1 {
		t.Fatalf("trait callback ran %d times", adminAfter)
	}
}

func TestUnknownTrait(t *testing.T) {
	r := New()
	mustDefine(t, r, "user", func(d *Definition) { d.Attr("a", 1) })
	if _, err := r.Build("user", WithTrait("ghost")); !errors.Is(err, ErrUnknownTrait) {
		t.Fatalf("err = %v, want ErrUnknownTrait", err)
	}
}

// --- transient + overrides --------------------------------------------------

func TestTransientAndOverrides(t *testing.T) {
	r := New()
	mustDefine(t, r, "user", func(d *Definition) {
		d.Transient(func(tr *Definition) {
			tr.Attr("upcase", false)
		})
		d.Dynamic("name", func(e *Evaluator) any {
			if e.Get("upcase").(bool) {
				return "ADA"
			}
			return "ada"
		})
	})
	// default transient
	o1, _ := r.Build("user")
	m1 := asMap(t, o1)
	if m1["name"] != "ada" {
		t.Fatalf("name = %v", m1["name"])
	}
	if _, leaked := m1["upcase"]; leaked {
		t.Fatalf("transient leaked into object: %v", m1)
	}
	// override transient (flag preserved -> still excluded from object)
	o2, _ := r.Build("user", With("upcase", true))
	m2 := asMap(t, o2)
	if m2["name"] != "ADA" {
		t.Fatalf("overridden name = %v", m2["name"])
	}
	if _, leaked := m2["upcase"]; leaked {
		t.Fatalf("overridden transient leaked: %v", m2)
	}
	// override a normal attribute + inject a brand-new one
	o3, _ := r.Build("user", WithAttrs(map[string]any{"name": "grace", "extra": 7}))
	m3 := asMap(t, o3)
	if m3["name"] != "grace" || m3["extra"] != 7 {
		t.Fatalf("override map = %v", m3)
	}
}

// --- attributes_for: excludes transient and associations, no callbacks ------

func TestAttributesFor(t *testing.T) {
	r := New()
	built := 0
	cbRan := false
	mustDefine(t, r, "account", func(d *Definition) { d.Attr("kind", "free") })
	mustDefine(t, r, "user", func(d *Definition) {
		d.Attr("name", "ada")
		d.Transient(func(tr *Definition) { tr.Attr("secret", "x") })
		d.Association("account", "account", nil)
		d.AfterBuild(func(obj any, e *Evaluator) error { cbRan = true; return nil })
	})
	r.SetBuild(func(class string, attrs map[string]any) (any, error) { built++; return attrs, nil })

	attrs, err := r.AttributesFor("user")
	if err != nil {
		t.Fatalf("AttributesFor: %v", err)
	}
	if attrs["name"] != "ada" {
		t.Fatalf("name = %v", attrs["name"])
	}
	if _, ok := attrs["secret"]; ok {
		t.Fatalf("transient present: %v", attrs)
	}
	if _, ok := attrs["account"]; ok {
		t.Fatalf("association present: %v", attrs)
	}
	if built != 0 {
		t.Fatalf("attributes_for instantiated %d objects", built)
	}
	if cbRan {
		t.Fatalf("attributes_for ran callbacks")
	}
}

func TestAttributesForUnknownFactory(t *testing.T) {
	r := New()
	if _, err := r.AttributesFor("nope"); !errors.Is(err, ErrUnknownFactory) {
		t.Fatalf("err = %v", err)
	}
}

func TestAttributesForSequenceOverflow(t *testing.T) {
	r := New()
	mustDefine(t, r, "user", func(d *Definition) { d.SequenceFrom("id", math.MaxInt64, nil) })
	if _, err := r.AttributesFor("user"); !errors.Is(err, ErrSequenceOverflow) {
		t.Fatalf("err = %v", err)
	}
}

// --- associations: happy path, strategy propagation, cycle ------------------

func TestAssociationBuildAndCreate(t *testing.T) {
	r := New()
	persisted := map[string]int{}
	r.SetPersist(func(class string, obj any) error { persisted[class]++; return nil })
	mustDefine(t, r, "account", func(d *Definition) {
		d.Class("Account")
		d.Attr("plan", "free")
	})
	mustDefine(t, r, "user", func(d *Definition) {
		d.Class("User")
		d.Attr("name", "ada")
		d.Association("account", "", map[string]any{"plan": "pro"}) // empty factory -> name
	})

	// build strategy: association built, nothing persisted.
	bo, err := r.Build("user")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	acc := asMap(t, asMap(t, bo)["account"])
	if acc["plan"] != "pro" {
		t.Fatalf("assoc override not applied: %v", acc)
	}
	if len(persisted) != 0 {
		t.Fatalf("build persisted something: %v", persisted)
	}

	// create strategy: both user and account persisted.
	if _, err := r.Create("user"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if persisted["User"] != 1 || persisted["Account"] != 1 {
		t.Fatalf("persisted = %v", persisted)
	}
}

func TestAssociationWithTrait(t *testing.T) {
	r := New()
	mustDefine(t, r, "account", func(d *Definition) {
		d.Attr("plan", "free")
		d.Trait("pro", func(tr *Definition) { tr.Attr("plan", "pro") })
	})
	mustDefine(t, r, "user", func(d *Definition) {
		d.Association("account", "account", nil, "pro")
	})
	obj, err := r.Build("user")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	acc := asMap(t, asMap(t, obj)["account"])
	if acc["plan"] != "pro" {
		t.Fatalf("assoc trait not applied: %v", acc)
	}
}

func TestAssociationCycle(t *testing.T) {
	r := New()
	mustDefine(t, r, "a", func(d *Definition) { d.Association("b", "b", nil) })
	mustDefine(t, r, "b", func(d *Definition) { d.Association("a", "a", nil) })
	if _, err := r.Build("a"); !errors.Is(err, ErrAssociationCycle) {
		t.Fatalf("err = %v, want ErrAssociationCycle", err)
	}
}

func TestAssociationUnknownTarget(t *testing.T) {
	r := New()
	mustDefine(t, r, "user", func(d *Definition) { d.Association("account", "account", nil) })
	if _, err := r.Build("user"); !errors.Is(err, ErrUnknownFactory) {
		t.Fatalf("err = %v", err)
	}
}

// --- parent / child inheritance + nested factories --------------------------

func TestParentInheritance(t *testing.T) {
	r := New()
	mustDefine(t, r, "person", func(d *Definition) {
		d.Class("Person")
		d.Attr("species", "human")
		d.Attr("role", "none")
		d.Trait("vip", func(tr *Definition) { tr.Attr("role", "vip") })
	})
	mustDefine(t, r, "admin", func(d *Definition) {
		d.Parent("person")
		d.Attr("role", "admin") // override parent
		d.Attr("level", 9)      // new
	})
	var class string
	r.SetBuild(func(c string, a map[string]any) (any, error) { class = c; return a, nil })
	obj, err := r.Build("admin")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	m := asMap(t, obj)
	if m["species"] != "human" || m["role"] != "admin" || m["level"] != 9 {
		t.Fatalf("inherited attrs = %v", m)
	}
	if class != "Person" {
		t.Fatalf("child should inherit parent class, got %q", class)
	}
	// inherited trait applies to child too.
	vip, _ := r.Build("admin", WithTrait("vip"))
	if asMap(t, vip)["role"] != "vip" {
		t.Fatalf("inherited trait not applied: %v", asMap(t, vip))
	}
}

func TestNestedFactories(t *testing.T) {
	r := New()
	mustDefine(t, r, "user", func(d *Definition) {
		d.Attr("role", "member")
		d.Factory("admin", func(f *Definition) {
			f.Attr("role", "admin")
			f.Factory("superadmin", func(g *Definition) { g.Attr("level", 99) })
		})
	})
	adm, err := r.Build("admin")
	if err != nil {
		t.Fatalf("Build admin: %v", err)
	}
	if asMap(t, adm)["role"] != "admin" {
		t.Fatalf("admin role = %v", asMap(t, adm)["role"])
	}
	sup, err := r.Build("superadmin")
	if err != nil {
		t.Fatalf("Build superadmin: %v", err)
	}
	m := asMap(t, sup)
	if m["role"] != "admin" || m["level"] != 99 {
		t.Fatalf("superadmin = %v", m)
	}
}

func TestNestedDuplicateName(t *testing.T) {
	r := New()
	err := r.Define("user", func(d *Definition) {
		d.Factory("user", func(f *Definition) {}) // clashes with parent name
	})
	if !errors.Is(err, ErrDuplicateFactory) {
		t.Fatalf("err = %v, want ErrDuplicateFactory", err)
	}
}

func TestParentCycle(t *testing.T) {
	r := New()
	mustDefine(t, r, "a", func(d *Definition) { d.Parent("b") })
	mustDefine(t, r, "b", func(d *Definition) { d.Parent("a") })
	if _, err := r.Build("a"); !errors.Is(err, ErrParentCycle) {
		t.Fatalf("err = %v, want ErrParentCycle", err)
	}
}

func TestUnknownParent(t *testing.T) {
	r := New()
	mustDefine(t, r, "a", func(d *Definition) { d.Parent("ghost") })
	if _, err := r.Build("a"); !errors.Is(err, ErrUnknownFactory) {
		t.Fatalf("err = %v", err)
	}
}

// --- callback pipeline + ordering + error branches --------------------------

func TestCallbackPipelineOrder(t *testing.T) {
	r := New()
	var order []string
	r.SetPersist(func(class string, obj any) error { order = append(order, "persist"); return nil })
	mustDefine(t, r, "user", func(d *Definition) {
		d.Attr("a", 1)
		d.AfterBuild(func(obj any, e *Evaluator) error { order = append(order, "after_build"); return nil })
		d.BeforeCreate(func(obj any, e *Evaluator) error { order = append(order, "before_create"); return nil })
		d.AfterCreate(func(obj any, e *Evaluator) error { order = append(order, "after_create"); return nil })
	})

	order = nil
	if _, err := r.Build("user"); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(order) != 1 || order[0] != "after_build" {
		t.Fatalf("build order = %v", order)
	}

	order = nil
	if _, err := r.Create("user"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	want := []string{"after_build", "before_create", "persist", "after_create"}
	if fmt.Sprint(order) != fmt.Sprint(want) {
		t.Fatalf("create order = %v, want %v", order, want)
	}
}

func TestCallbackReadsTransient(t *testing.T) {
	r := New()
	got := ""
	mustDefine(t, r, "user", func(d *Definition) {
		d.Transient(func(tr *Definition) { tr.Attr("note", "hi") })
		d.AfterBuild(func(obj any, e *Evaluator) error { got = e.Get("note").(string); return nil })
	})
	if _, err := r.Build("user"); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got != "hi" {
		t.Fatalf("callback transient = %q", got)
	}
}

func TestAfterBuildError(t *testing.T) {
	r := New()
	boom := errors.New("boom")
	mustDefine(t, r, "user", func(d *Definition) {
		d.AfterBuild(func(obj any, e *Evaluator) error { return boom })
	})
	if _, err := r.Build("user"); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}

func TestBeforeCreateError(t *testing.T) {
	r := New()
	boom := errors.New("boom")
	mustDefine(t, r, "user", func(d *Definition) {
		d.BeforeCreate(func(obj any, e *Evaluator) error { return boom })
	})
	if _, err := r.Create("user"); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}

func TestAfterCreateError(t *testing.T) {
	r := New()
	boom := errors.New("boom")
	mustDefine(t, r, "user", func(d *Definition) {
		d.AfterCreate(func(obj any, e *Evaluator) error { return boom })
	})
	if _, err := r.Create("user"); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}

// --- seam error branches ----------------------------------------------------

func TestBuildSeamError(t *testing.T) {
	r := New()
	boom := errors.New("no build")
	r.SetBuild(func(class string, attrs map[string]any) (any, error) { return nil, boom })
	mustDefine(t, r, "user", func(d *Definition) { d.Attr("a", 1) })
	if _, err := r.Build("user"); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}

func TestPersistSeamError(t *testing.T) {
	r := New()
	boom := errors.New("no save")
	r.SetPersist(func(class string, obj any) error { return boom })
	mustDefine(t, r, "user", func(d *Definition) { d.Attr("a", 1) })
	if _, err := r.Create("user"); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}

func TestDefaultPersistNoop(t *testing.T) {
	r := New()
	mustDefine(t, r, "user", func(d *Definition) { d.Attr("a", 1) })
	if _, err := r.Create("user"); err != nil {
		t.Fatalf("Create with default persist: %v", err)
	}
}

// --- unknown factory across entry points ------------------------------------

func TestUnknownFactory(t *testing.T) {
	r := New()
	if _, err := r.Build("nope"); !errors.Is(err, ErrUnknownFactory) {
		t.Fatalf("Build err = %v", err)
	}
	if _, err := r.Create("nope"); !errors.Is(err, ErrUnknownFactory) {
		t.Fatalf("Create err = %v", err)
	}
}

func TestDuplicateFactory(t *testing.T) {
	r := New()
	mustDefine(t, r, "user", func(d *Definition) {})
	if err := r.Define("user", func(d *Definition) {}); !errors.Is(err, ErrDuplicateFactory) {
		t.Fatalf("err = %v, want ErrDuplicateFactory", err)
	}
}

// --- lists ------------------------------------------------------------------

func TestBuildAndCreateList(t *testing.T) {
	r := New()
	persisted := 0
	r.SetPersist(func(class string, obj any) error { persisted++; return nil })
	mustDefine(t, r, "user", func(d *Definition) {
		d.Sequence("id", nil)
	})
	bl, err := r.BuildList("user", 3)
	if err != nil {
		t.Fatalf("BuildList: %v", err)
	}
	if len(bl) != 3 {
		t.Fatalf("len = %d", len(bl))
	}
	if asMap(t, bl[0])["id"].(int64) != 1 || asMap(t, bl[2])["id"].(int64) != 3 {
		t.Fatalf("ids = %v %v", asMap(t, bl[0])["id"], asMap(t, bl[2])["id"])
	}
	cl, err := r.CreateList("user", 2)
	if err != nil {
		t.Fatalf("CreateList: %v", err)
	}
	if len(cl) != 2 || persisted != 2 {
		t.Fatalf("create list len=%d persisted=%d", len(cl), persisted)
	}
	// zero-length lists.
	if l, _ := r.BuildList("user", 0); len(l) != 0 {
		t.Fatalf("expected empty build list")
	}
}

func TestBuildListError(t *testing.T) {
	r := New()
	if _, err := r.BuildList("nope", 2); !errors.Is(err, ErrUnknownFactory) {
		t.Fatalf("BuildList err = %v", err)
	}
	if _, err := r.CreateList("nope", 2); !errors.Is(err, ErrUnknownFactory) {
		t.Fatalf("CreateList err = %v", err)
	}
}

// --- default singleton registry --------------------------------------------

func TestDefaultRegistryAndReset(t *testing.T) {
	Reset()
	if err := Default().Define("thing", func(d *Definition) { d.Attr("a", 1) }); err != nil {
		t.Fatalf("Define: %v", err)
	}
	if _, err := Default().Build("thing"); err != nil {
		t.Fatalf("Build: %v", err)
	}
	Reset()
	if _, err := Default().Build("thing"); !errors.Is(err, ErrUnknownFactory) {
		t.Fatalf("after Reset err = %v, want ErrUnknownFactory", err)
	}
}
