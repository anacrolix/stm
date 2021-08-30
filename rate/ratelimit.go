package rate

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/anacrolix/stm"
	"github.com/anacrolix/stm/stmutil"
)

type numTokens = int

type Limiter struct {
	max     *stm.Var[numTokens]
	cur     *stm.Var[numTokens]
	lastAdd *stm.Var[time.Time]
	rate    Limit
}

const Inf = Limit(math.MaxFloat64)

type Limit float64

func (l Limit) interval() time.Duration {
	return time.Duration(Limit(1*time.Second) / l)
}

func Every(interval time.Duration) Limit {
	if interval == 0 {
		return Inf
	}
	return Limit(time.Second / interval)
}

func NewLimiter(rate Limit, burst numTokens) *Limiter {
	rl := &Limiter{
		max:     stm.NewVar(burst),
		cur:     stm.NewBuiltinEqVar(burst),
		lastAdd: stm.NewVar(time.Now()),
		rate:    rate,
	}
	if rate != Inf {
		go rl.tokenGenerator(rate.interval())
	}
	return rl
}

func (rl *Limiter) tokenGenerator(interval time.Duration) {
	for {
		lastAdd := stm.AtomicGet(rl.lastAdd)
		time.Sleep(time.Until(lastAdd.Add(interval)))
		now := time.Now()
		available := numTokens(now.Sub(lastAdd) / interval)
		if available < 1 {
			continue
		}
		stm.Atomically(stm.VoidOperation(func(tx *stm.Tx) {
			cur := rl.cur.Get(tx)
			max := rl.max.Get(tx)
			tx.Assert(cur < max)
			newCur := cur + available
			if newCur > max {
				newCur = max
			}
			if newCur != cur {
				rl.cur.Set(tx, newCur)
			}
			rl.lastAdd.Set(tx, lastAdd.Add(interval*time.Duration(available)))
		}))
	}
}

func (rl *Limiter) Allow() bool {
	return rl.AllowN(1)
}

func (rl *Limiter) AllowN(n numTokens) bool {
	return stm.Atomically(func(tx *stm.Tx) interface{} {
		return rl.takeTokens(tx, n)
	}).(bool)
}

func (rl *Limiter) AllowStm(tx *stm.Tx) bool {
	return rl.takeTokens(tx, 1)
}

func (rl *Limiter) takeTokens(tx *stm.Tx, n numTokens) bool {
	if rl.rate == Inf {
		return true
	}
	cur := rl.cur.Get(tx)
	if cur >= n {
		rl.cur.Set(tx, cur-n)
		return true
	}
	return false
}

func (rl *Limiter) Wait(ctx context.Context) error {
	return rl.WaitN(ctx, 1)
}

func (rl *Limiter) WaitN(ctx context.Context, n int) error {
	ctxDone, cancel := stmutil.ContextDoneVar(ctx)
	defer cancel()
	if err := stm.Atomically(func(tx *stm.Tx) interface{} {
		if ctxDone.Get(tx) {
			return ctx.Err()
		}
		if rl.takeTokens(tx, n) {
			return nil
		}
		if n > rl.max.Get(tx) {
			return errors.New("burst exceeded")
		}
		if dl, ok := ctx.Deadline(); ok {
			if rl.cur.Get(tx)+numTokens(dl.Sub(rl.lastAdd.Get(tx))/rl.rate.interval()) < n {
				return context.DeadlineExceeded
			}
		}
		tx.Retry()
		panic("unreachable")
	}); err != nil {
		return err.(error)
	}
	return nil

}
