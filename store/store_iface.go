package store

import (
	"github.com/grokify/omniroadmap/compile"
	"github.com/grokify/omniroadmap/materialize"
	"github.com/grokify/omniroadmap/review"
	"github.com/grokify/omniroadmap/sync"
)

// Compile-time checks that DoltStore satisfies each consumer package's own
// Store contract (sync, compile, review, materialize each define theirs
// independently for testability against fakes — see each package's Store
// interface).
var (
	_ sync.Store        = (*DoltStore)(nil)
	_ compile.Store     = (*DoltStore)(nil)
	_ review.Store      = (*DoltStore)(nil)
	_ materialize.Store = (*DoltStore)(nil)
)
