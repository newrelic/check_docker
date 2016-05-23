package main

import (
	"flag"
	"github.com/stretchr/testify/assert"
	"testing"
)

// Make sure that, if our flag value appears on the command line, the value is
// captured and correctly stored
//
func TestParseMultiStringArgOne(t *testing.T) {
	var m multiStringArg

	flags := flag.NewFlagSet("test", flag.PanicOnError)
	flags.Var(&m, "test", "My Flag")
	flags.Parse([]string{"--test", "working!"})

	assert.Len(t, m, 1)
	assert.Equal(t, m[0], "working!")
}

// We should handle arguments like --test=awesome correctly as well
//
func TestParseMultiStringArgWithEquals(t *testing.T) {
	var m multiStringArg

	flags := flag.NewFlagSet("test", flag.PanicOnError)
	flags.Var(&m, "test", "My Flag")
	flags.Parse([]string{"--test=awesome"})

	assert.Len(t, m, 1)
	assert.Equal(t, m[0], "awesome")
}

// Make sure we correctly process multiple arguments
//
func TestParseMultiStringArgMulti(t *testing.T) {
	var m multiStringArg

	flags := flag.NewFlagSet("test", flag.PanicOnError)
	flags.Var(&m, "test", "My Flag")
	flags.Parse([]string{"--test=awesome", "--test", "great", "--test", "banging!"})

	assert.Len(t, m, 3)
	assert.Equal(t, m[0], "awesome")
	assert.Equal(t, m[1], "great")
	assert.Equal(t, m[2], "banging!")
}

// If we get the empty string, we should add it to the argument list
//
func TestParseMultiStringArgHandleEmptyString(t *testing.T) {
	var m multiStringArg

	flags := flag.NewFlagSet("test", flag.PanicOnError)
	flags.Var(&m, "test", "My Flag")
	flags.Parse([]string{"--test=", "--test=great", "--test", "banging!"})

	assert.Len(t, m, 3)
	assert.Equal(t, m[0], "")
	assert.Equal(t, m[1], "great")
	assert.Equal(t, m[2], "banging!")
}

// Make sure we don't capture arguments intended for other flags
//
func TestParseMultiStringArgIgnoreOther(t *testing.T) {
	var m multiStringArg

	flags := flag.NewFlagSet("test", flag.PanicOnError)
	flags.Var(&m, "test", "My Flag")
	flags.String("other", "", "blah")
	flags.Parse([]string{"--test=awesome", "--other=good", "--test", "great"})

	assert.Len(t, m, 2)
	assert.Equal(t, m[0], "awesome")
	assert.Equal(t, m[1], "great")
}

// If no relevant arguments are supplied, our variable should have len 0
//
func TestParseMultiStringArgNotUsed(t *testing.T) {
	var m multiStringArg

	flags := flag.NewFlagSet("test", flag.PanicOnError)
	flags.String("other", "", "Blah")
	flags.Var(&m, "test", "My Flag")
	flags.Parse([]string{"--other=good"})

	assert.Len(t, m, 0)
}
