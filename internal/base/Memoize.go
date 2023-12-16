package base

import "sync"

/***************************************
 * Memoize
 ***************************************/

func Memoize[T any](fn func() T) func() T {
	var memoized T
	once := sync.Once{}
	return func() T {
		once.Do(func() { memoized = fn() })
		return memoized
	}
}

func MemoizeComparable[T any, ARG comparable](fn func(ARG) T) func(ARG) T {
	memoized := make(map[ARG]T)
	mutex := sync.Mutex{}

	var lastArg ARG
	var lastRet T
	return func(a ARG) T {
		mutex.Lock()
		defer mutex.Unlock()

		if lastArg == a {
			return lastRet
		}

		result, ok := memoized[a]
		if !ok {
			result = fn(a)
			memoized[a] = result
		}

		lastArg = a
		lastRet = result
		return result
	}
}

type memoized_equatable_arg[T any, ARG Equatable[ARG]] struct {
	key   ARG
	value T
}

func MemoizeEquatable[T any, ARG Equatable[ARG]](fn func(ARG) T) func(ARG) T {
	memoized := make([]memoized_equatable_arg[T, ARG], 0, 1)
	mutex := sync.Mutex{}
	return func(a ARG) T {
		mutex.Lock()
		defer mutex.Unlock()

		for _, it := range memoized {
			if it.key.Equals(a) {
				return it.value
			}
		}

		result := fn(a)
		memoized = append(memoized, memoized_equatable_arg[T, ARG]{key: a, value: result})
		return result
	}
}
