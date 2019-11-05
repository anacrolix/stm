package stmutil

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContextEquality(t *testing.T) {
	ctx := context.Background()
	assert.True(t, ctx == context.Background())
	childCtx, cancel := context.WithCancel(ctx)
	assert.True(t, childCtx != ctx)
	assert.True(t, childCtx != ctx)
	assert.Equal(t, context.Background(), ctx)
	cancel()
	assert.Equal(t, context.Background(), ctx)
	assert.NotEqual(t, ctx, childCtx)
}
