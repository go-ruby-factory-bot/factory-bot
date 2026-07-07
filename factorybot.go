// Copyright (c) the go-ruby-factory-bot/factory-bot authors
//
// SPDX-License-Identifier: BSD-3-Clause

package factorybot

// std is the process-wide default registry, mirroring the FactoryBot module's
// singleton registry that `FactoryBot.define` / `FactoryBot.build` operate on.
var std = New()

// Default returns the process-wide default registry, the one a future rbgo
// binding maps the FactoryBot module onto.
func Default() *Registry { return std }

// Reset clears the default registry, mirroring FactoryBot.reload for tests.
func Reset() { *std = *New() }
