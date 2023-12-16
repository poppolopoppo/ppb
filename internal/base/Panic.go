package base

import "fmt"

type PanicResult int32

const (
	PANIC_ABORT PanicResult = iota
	PANIC_HANDLED
	PANIC_REENTRANCY
)

var OnPanic func(error) PanicResult

func Panicf(msg string, args ...interface{}) {
	Panic(fmt.Errorf(msg, args...))
}

func Panic(err error) {
	result := PANIC_ABORT
	if OnPanic != nil {
		result = OnPanic(err)
	}

	PurgePinnedLogs()

	switch result {
	case PANIC_ABORT:
		panic(fmt.Errorf("%v%v%v[PANIC]%v %v",
			ANSI_FG1_RED, ANSI_BG1_WHITE, ANSI_BLINK0, ANSI_RESET, err))
	case PANIC_HANDLED:
		LogError(LogBase, "handled panic: %v", err)
	case PANIC_REENTRANCY:
		panic(fmt.Errorf("panic reentrancy: %v", err))
	}
}
