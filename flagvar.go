package main

import (
	"strings"
)

type multiStringArg []string

func (m *multiStringArg) String() string {
	return strings.Join([]string(*m), " ")
}

func (m *multiStringArg) Set(val string) error {
	*m = append(*m, val)
	return nil
}
