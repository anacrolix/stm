package stm

// Atomically executes the atomic function fn.
func Atomically(fn func(*Tx)) {
retry:
	// run the transaction
	tx := &Tx{
		reads:  make(map[*Var]uint64),
		writes: make(map[*Var]interface{}),
	}
	tx.cond.L = &globalLock
	if catchRetry(fn, tx) {
		// wait for one of the variables we read to change before retrying
		tx.wait()
		goto retry
	}
	// verify the read log
	globalLock.Lock()
	if !tx.verify() {
		globalLock.Unlock()
		goto retry
	}
	// commit the write log and broadcast that variables have changed
	if len(tx.writes) > 0 {
		tx.commit()
		globalCond.Broadcast()
	}
	globalLock.Unlock()
}

// AtomicGet is a helper function that atomically reads a value.
func AtomicGet(v *Var) interface{} {
	// since we're only doing one operation, we don't need a full transaction
	globalLock.Lock()
	v.mu.Lock()
	val := v.val
	v.mu.Unlock()
	globalLock.Unlock()
	return val
}

// AtomicSet is a helper function that atomically writes a value.
func AtomicSet(v *Var, val interface{}) {
	Atomically(func(tx *Tx) {
		tx.Set(v, val)
	})
}

// Compose is a helper function that composes multiple transactions into a
// single transaction.
func Compose(fns ...func(*Tx)) func(*Tx) {
	return func(tx *Tx) {
		for _, f := range fns {
			f(tx)
		}
	}
}

// Select runs the supplied functions in order. Execution stops when a
// function succeeds without calling Retry. If no functions succeed, the
// entire selection will be retried.
func Select(fns ...func(*Tx)) func(*Tx) {
	return func(tx *Tx) {
		switch len(fns) {
		case 0:
			// empty Select blocks forever
			tx.Retry()
		case 1:
			fns[0](tx)
		default:
			if catchRetry(fns[0], tx) {
				Select(fns[1:]...)(tx)
			}
		}
	}
}
