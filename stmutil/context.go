package stmutil

import (
	"context"
	"sync"

	"github.com/anacrolix/stm"
)

var (
	mu      sync.Mutex
	ctxVars = map[context.Context]*stm.Var{}
)

func ContextDoneVar(ctx context.Context) (*stm.Var, func()) {
	mu.Lock()
	defer mu.Unlock()
	if v, ok := ctxVars[ctx]; ok {
		return v, func() {}
	}
	if ctx.Err() != nil {
		v := stm.NewVar(true)
		ctxVars[ctx] = v
		return v, func() {}
	}
	v := stm.NewVar(false)
	go func() {
		<-ctx.Done()
		stm.AtomicSet(v, true)
	}()
	ctxVars[ctx] = v
	return v, func() {}
}
