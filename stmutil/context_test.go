package stmutil

import (
	"context"
	"testing"

	qt "github.com/go-quicktest/qt"
)

func TestContextEquality(t *testing.T) {
	ctx := context.Background()
	qt.Check(t, qt.IsTrue(ctx == context.Background()))
	childCtx, cancel := context.WithCancel(ctx)
	qt.Check(t, qt.IsTrue(childCtx != ctx))
	qt.Check(t, qt.IsTrue(childCtx != ctx))
	qt.Check(t, qt.Equals(ctx, context.Background()))
	cancel()
	qt.Check(t, qt.Equals(ctx, context.Background()))
	qt.Check(t, qt.Not(qt.Equals(childCtx, ctx)))
}
