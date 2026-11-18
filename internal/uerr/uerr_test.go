package uerr

import (
	"errors"
	"fmt"
	"testing"
)

func TestNewfFillsTextAndArgs(t *testing.T) {
	err := Newf("location.exists", "location %q already exists", "name", "Den")
	if err.Error() != `location "Den" already exists` {
		t.Fatalf("text: %q", err.Error())
	}
	if err.Code != "location.exists" || err.Args["name"] != "Den" {
		t.Fatalf("code/args: %+v", err)
	}
}

func TestOfUnwraps(t *testing.T) {
	inner := New("group.empty", "group name cannot be empty")
	wrapped := fmt.Errorf("saving: %w", inner)
	got, ok := Of(wrapped)
	if !ok || got.Code != "group.empty" {
		t.Fatalf("Of(wrapped) = %v, %v", got, ok)
	}
	if _, ok := Of(errors.New("plain")); ok {
		t.Fatal("plain error should not match")
	}
}
