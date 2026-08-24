//go:build tools

// Package ent: this file pins the ent code generator's dependencies so
// `go mod tidy` doesn't strip them (the generator is only referenced from
// the //go:generate directive in generate.go, which tidy can't see). Never
// built — the tools tag is never satisfied.
package ent

import (
	_ "entgo.io/ent/cmd/ent"
)
