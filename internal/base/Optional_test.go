package base

import (
	"errors"
	"testing"
)

func TestNewOption(t *testing.T) {
	opt := NewOption(42)
	if !opt.Valid() {
		t.Errorf("Expected option to be valid")
	}
	val, err := opt.Get()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if val != 42 {
		t.Errorf("Expected value 42, got %v", val)
	}
}

func TestNoneOption(t *testing.T) {
	opt := NoneOption[int]()
	if opt.Valid() {
		t.Errorf("Expected option to be invalid")
	}
	_, err := opt.Get()
	if !errors.Is(err, ErrEmptyOptional) {
		t.Errorf("Expected ErrEmptyOptional, got %v", err)
	}
}

func TestUnexpectedOption(t *testing.T) {
	myErr := errors.New("my error")
	opt := UnexpectedOption[int](myErr)
	if opt.Valid() {
		t.Errorf("Expected option to be invalid")
	}
	_, err := opt.Get()
	if !errors.Is(err, myErr) {
		t.Errorf("Expected myErr, got %v", err)
	}
}

func TestGetOrElse(t *testing.T) {
	opt := NewOption("hello")
	if got := opt.GetOrElse("world"); got != "hello" {
		t.Errorf("Expected 'hello', got %v", got)
	}
	optNone := NoneOption[string]()
	if got := optNone.GetOrElse("world"); got != "world" {
		t.Errorf("Expected 'world', got %v", got)
	}
}

func TestWhatIf(t *testing.T) {
	opt := NewOption(10)
	called := false
	err := opt.WhatIf(func(v *int) error {
		called = true
		*v = 20
		return nil
	})
	if err != nil {
		t.Errorf("Expected nil error, got %v", err)
	}
	if !called {
		t.Errorf("Expected function to be called")
	}
	val, _ := opt.Get()
	if val != 20 {
		t.Errorf("Expected value 20, got %v", val)
	}

	optNone := NoneOption[int]()
	called = false
	_ = optNone.WhatIf(func(v *int) error {
		called = true
		return nil
	})
	if called {
		t.Errorf("Expected function not to be called")
	}
}

type testSettable int

func (t *testSettable) Set(s string) error {
	if s == "fail" {
		return errors.New("fail")
	}
	*t = testSettable(len(s))
	return nil
}

func TestSetOptional(t *testing.T) {
	var opt Optional[testSettable]
	err := SetOptional("abc", &opt)
	if err != nil {
		t.Errorf("Expected nil error, got %v", err)
	}
	val, err := opt.Get()
	if err != nil {
		t.Errorf("Expected nil error, got %v", err)
	}
	if val != testSettable(3) {
		t.Errorf("Expected value 3, got %v", val)
	}

	err = SetOptional("fail", &opt)
	if err == nil {
		t.Errorf("Expected error, got nil")
	}
}
