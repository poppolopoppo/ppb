package base

import "errors"

/***************************************
 * Optional[T] holds a value or an error, with value semantics
 ***************************************/

type Optional[T any] struct {
	value T
	err   error
}

type optionEmptyError struct{}

func (x optionEmptyError) Error() string {
	return "option has no value"
}

var ErrEmptyOptional error = errors.New("empty optional")

func NewOption[T any](value T) Optional[T] {
	return Optional[T]{
		value: value,
	}
}
func NoneOption[T any]() Optional[T] {
	return UnexpectedOption[T](ErrEmptyOptional)
}
func UnexpectedOption[T any](err error) Optional[T] {
	return Optional[T]{
		err: err,
	}
}

func (x Optional[T]) Valid() bool {
	return x.err == nil
}
func (x Optional[T]) Get() (T, error) {
	return x.value, x.err

}
func (x Optional[T]) GetOrElse(orElse T) T {
	if x.err == nil {
		return x.value
	} else {
		return orElse
	}
}
