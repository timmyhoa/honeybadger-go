package honeybadger

import (
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"strconv"
)

const maxFrames = 20

// Frame represent a stack frame inside of a Honeybadger backtrace.
type Frame struct {
	Number string `json:"number"`
	File   string `json:"file"`
	Method string `json:"method"`
}

// Error provides more structured information about a Go error.
type Error struct {
	err     interface{}
	Message string
	Class   string
	Stack   []*Frame
}

func (e Error) Error() string {
	return e.Message
}

func NewError(msg interface{}) Error {
	return newError(msg, 2)
}

func NewErrorWithCustomOffset(msg interface{}, stackOffset int) Error {
	return newError(msg, stackOffset)
}

func newError(thing interface{}, stackOffset int) Error {
	var err error
	assertedError, ok := thing.(error)

	if ok {
		if errors.As(assertedError, &Error{}) {
			return assertedError.(Error)
		} else {
			err = assertedError
		}
	} else {
		err = fmt.Errorf("%v", assertedError)
	}

	return Error{
		err:     err,
		Message: err.Error(),
		Class:   reflect.TypeOf(err).String(),
		Stack:   generateStack(stackOffset),
	}
}

func generateStack(offset int) []*Frame {
	stack := make([]uintptr, maxFrames)
	length := runtime.Callers(2+offset, stack[:])

	frames := runtime.CallersFrames(stack[:length])
	result := make([]*Frame, 0, length)

	for {
		frame, more := frames.Next()

		result = append(result, &Frame{
			File:   frame.File,
			Number: strconv.Itoa(frame.Line),
			Method: frame.Function,
		})

		if !more {
			break
		}
	}

	return result
}
