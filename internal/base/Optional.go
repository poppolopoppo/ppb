package base

type Optional[T any] interface {
	Valid() bool
	Get() (T, error)
	GetOrElse(orElse T) T
}

type optionEmptyError struct{}

func (x optionEmptyError) Error() string {
	return "option has no value"
}

var OptionEmptyError error = optionEmptyError{}

type Option[T any] struct {
	value T
	valid bool
}

func NewOption[T any](value T) Option[T] {
	return Option[T]{
		value: value,
		valid: true,
	}
}

func (x Option[T]) Valid() bool {
	return x.valid
}
func (x Option[T]) Get() (T, error) {
	if x.valid {
		return x.value, nil
	} else {
		return x.value, OptionEmptyError
	}
}
func (x Option[T]) GetOrElse(orElse T) T {
	if x.valid {
		return x.value
	} else {
		return orElse
	}
}

type None[T any] struct{}

func (x None[T]) Valid() bool               { return false }
func (x None[T]) Get() (value T, err error) { err = OptionEmptyError; return }
func (x None[T]) GetOrElse(orElse T) T      { return orElse }
