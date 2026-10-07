<p align="center"><img src="https://go-ruby-factory-bot.github.io/logo.png" alt="go-ruby-factory-bot/factory-bot" width="720"></p>

# factory-bot — go-ruby-factory-bot

[![Docs](https://img.shields.io/badge/docs-mkdocs--material-DC2626)](https://go-ruby-factory-bot.github.io/docs/)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.27.1%2B-00ADD8)](https://go.dev/dl/)
[![Coverage](https://img.shields.io/badge/coverage-100%25-1a7f37)](#tests--coverage)

**A pure-Go (no cgo) reimplementation of the deterministic core of Ruby's
[`factory_bot`](https://github.com/thoughtbot/factory_bot) gem** — the
fixtures-replacement library used to *define factories* and *construct objects*
for tests and seed data. It reproduces the factory registry, attribute
resolution, sequences, traits, associations, transient attributes, parent/child
inheritance, nested factories, and the build/create callback pipeline — **without
any Ruby runtime**.

It is the factory_bot engine for
[go-embedded-ruby](https://github.com/go-embedded-ruby/ruby), but a **standalone,
reusable** module.

> **What it is — and isn't.** Everything factory_bot does to resolve a factory
> into a bag of attributes is deterministic and needs **no interpreter**, so it
> lives here as pure Go: merging parent attributes, overlaying traits, evaluating
> static and dynamic (lazy) values, advancing sequences, resolving associations,
> separating transient attributes, and driving the callback pipeline in
> factory_bot's order (`after(:build)` → `before(:create)` → persist →
> `after(:create)`). The two things factory_bot delegates to the object model —
> **instantiating the class** and **persisting it** — are **host seams**:
> `BuildFunc(class, attrs)` builds the object (factory_bot's `initialize_with` +
> attribute assignment) and `PersistFunc(class, obj)` saves it (`to_create`,
> default `save!`). A dynamic attribute value is the block seam `Block`
> (`func(*Evaluator) any`), mirroring `attr { ... }`. **The default seams return
> the attribute map and a no-op**, so the whole engine is exercised with no ORM.
> A future rbgo binding wires the seams to Ruby object construction and
> ActiveRecord.

## Features

Faithful port of factory_bot's definition + resolution core:

- **Registry** — `New()`; `Define(name, func(*Definition))` mirrors
  `FactoryBot.define { factory :user do … end }`. `Default()` exposes a
  process-wide singleton (the `FactoryBot` module) and `Reset()` reloads it.
- **Attributes** — static (`Attr`), dynamic/lazy (`Dynamic`, the `Block` seam,
  memoized per build and able to read siblings via the `Evaluator`).
- **Sequences** — per-factory (`Sequence`/`SequenceFrom`) and global
  (`Registry.Sequence` + `Generate`), each a shared, monotonically increasing
  counter with an optional generator.
- **Traits** — named attribute + callback overlays (`Trait`), applied at build
  time with `WithTrait("admin")`; inherited from parents.
- **Associations** — `Association(name, factory, overrides, traits…)`, resolved
  by building/creating the referenced factory **with the parent strategy**
  (build within build, create within create), with **cycle detection**.
- **Transient attributes** — `Transient`, available to other attributes and
  callbacks through the evaluator but never passed to the `BuildFunc` seam.
- **Inheritance & nesting** — `Parent(name)` and nested `Factory(name, …)`;
  child attributes/traits/callbacks/class inherit and override, with
  parent-cycle detection.
- **Callbacks** — `AfterBuild` / `BeforeCreate` / `AfterCreate`, run in
  factory_bot's order; any callback error aborts the build.
- **Strategies** — `Build` / `Create` / `AttributesFor` / `BuildList` /
  `CreateList`, plus `With` / `WithAttrs` runtime overrides.
- **Seams** — `SetBuild(BuildFunc)` and `SetPersist(PersistFunc)`; the core
  never touches an object model or a database itself.
- **Error tree** — `errors.Is`-matchable sentinels: `ErrUnknownFactory`,
  `ErrUnknownTrait`, `ErrUnknownSequence`, `ErrSequenceOverflow`,
  `ErrDuplicateFactory`, `ErrParentCycle`, `ErrAssociationCycle`.

CGO-free, dependency-free (stdlib only), **100% test coverage**, `gofmt` +
`go vet` clean, and green across the six 64-bit Go targets (amd64, arm64,
riscv64, loong64, ppc64le, **s390x** — big-endian) plus `js/wasm` and
`wasip1/wasm`.

## Install

```sh
go get github.com/go-ruby-factory-bot/factory-bot
```

## Usage

```go
package main

import (
	"fmt"

	factorybot "github.com/go-ruby-factory-bot/factory-bot"
)

func main() {
	r := factorybot.New()

	_ = r.Define("account", func(d *factorybot.Definition) {
		d.Class("Account")
		d.Attr("plan", "free")
		d.Trait("pro", func(t *factorybot.Definition) { t.Attr("plan", "pro") })
	})

	_ = r.Define("user", func(d *factorybot.Definition) {
		d.Class("User")
		d.Sequence("email", func(n int) any { return fmt.Sprintf("user%d@example.com", n) })
		d.Attr("first", "Ada")
		d.Dynamic("greeting", func(e *factorybot.Evaluator) any {
			return "Hi " + e.Get("first").(string)
		})
		d.Transient(func(t *factorybot.Definition) { t.Attr("admin", false) })
		d.Association("account", "account", nil, "pro")

		d.AfterBuild(func(obj any, e *factorybot.Evaluator) error {
			// obj is whatever BuildFunc returned; e reads transients.
			return nil
		})

		d.Factory("admin_user", func(f *factorybot.Definition) { // nested, inherits user
			f.Attr("first", "Grace")
		})
	})

	u, _ := r.Build("user")                                   // build(:user)
	admin, _ := r.Create("admin_user", factorybot.With("first", "G")) // create(:admin_user, first: "G")
	list, _ := r.BuildList("user", 3)                         // build_list(:user, 3)
	attrs, _ := r.AttributesFor("user")                       // attributes_for(:user)

	fmt.Println(u, admin, len(list), attrs["greeting"])
}
```

### Injecting the object-model and persistence seams (hosts / rbgo)

```go
r := factorybot.New()
r.SetBuild(func(class string, attrs map[string]any) (any, error) {
	return myORM.New(class, attrs) // initialize_with + attribute assignment
})
r.SetPersist(func(class string, obj any) error {
	return myORM.Save(obj) // to_create (default save!)
})
```

## Value model

| gem                                         | this package                                        |
| ------------------------------------------- | --------------------------------------------------- |
| `FactoryBot.define`                         | `Registry.Define(name, func(*Definition))`          |
| `factory :user, class:, parent: do … end`   | `d.Class(...)` / `d.Parent(...)` / nested `d.Factory`|
| `name "x"` / `name { … }`                   | `d.Attr(name, x)` / `d.Dynamic(name, Block)`        |
| `sequence(:email) { |n| … }`                | `d.Sequence(name, gen)` / `Registry.Sequence`       |
| `trait :admin do … end`                     | `d.Trait(name, func(*Definition))`                  |
| `association :account`                      | `d.Association(name, factory, overrides, traits…)`  |
| `transient do … end`                        | `d.Transient(func(*Definition))`                    |
| `after(:build)` / `before/after(:create)`   | `d.AfterBuild` / `d.BeforeCreate` / `d.AfterCreate` |
| `build/create/attributes_for/*_list`        | `Build/Create/AttributesFor/BuildList/CreateList`   |
| `build(:user, :admin, name: "x")`           | `Build("user", WithTrait("admin"), With("name","x"))`|
| `initialize_with` / attribute assignment    | `BuildFunc` seam (`SetBuild`)                       |
| `to_create` (default `save!`)               | `PersistFunc` seam (`SetPersist`)                  |
| `FactoryBot::Errors` subtree                | `Err*` sentinels (`errors.Is`)                      |

## Tests & coverage

The suite is deterministic and pure: the default `BuildFunc`/`PersistFunc` seams
(map-returning / no-op) and stub seams drive every branch — factory build/create,
sequences (including overflow), traits, associations (including cycles),
transient attributes, inheritance, nested factories, the full callback pipeline,
and every error sentinel. **No test needs an ORM or a database**, so the
cross-arch qemu lanes and the Windows lane all hold coverage at **100%**.

```sh
COVERPKG=$(go list ./... | paste -sd, -)
go test -race -coverpkg="$COVERPKG" -coverprofile=cover.out ./...
go tool cover -func=cover.out | tail -1   # 100.0%
```

## WebAssembly

Being pure Go (CGO=0), this library also compiles to **WebAssembly** — both
`GOOS=js GOARCH=wasm` (browser / Node.js) and `GOOS=wasip1 GOARCH=wasm` (WASI).
CI builds both targets on every push, alongside the six 64-bit native/qemu arches.

```sh
GOOS=js     GOARCH=wasm go build ./...   # browser / Node
GOOS=wasip1 GOARCH=wasm go build ./...   # WASI (wasmtime, wasmer, wasmedge, …)
```

## License

BSD-3-Clause — see [LICENSE](LICENSE). Copyright the go-ruby-factory-bot/factory-bot authors.
