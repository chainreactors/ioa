package ioa

import "fmt"

type Error struct {
	Status int
	Detail string
}

func (e *Error) Error() string {
	return e.Detail
}

func ProtocolError(status int, format string, args ...interface{}) *Error {
	return &Error{Status: status, Detail: fmt.Sprintf(format, args...)}
}
