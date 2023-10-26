Package assert
==============

[![Build Status](https://travis-ci.org/go-playground/assert.svg?branch=master)](https://travis-ci.org/go-playground/assert)
[![GoDoc](https://godoc.org/github.com/go-playground/assert?status.svg)](https://godoc.org/gopkg.in/go-playground/assert.v1)

Package assert is a Basic Assertion library used along side native go testing

Installation
------------

Use go get.

	go get github.com/go-playground/assert

Then import the assert package into your own code.

	import . "github.com/go-playground/assert/v2"

Usage and documentation
------

Please see http://godoc.org/github.com/go-playground/assert for detailed usage docs.

##### Example:
```go
package whatever

import (
	"errors"
	"testing"
	. "github.com/go-playground/assert/v2"
)

func AssertCustomErrorHandler(t testing.TB, errs map[string]string, key, expected string) {
	val, ok := errs[key]

	// using EqualSkip and NotEqualSkip as building blocks for my custom Assert function
	EqualSkip(t, 2, ok, true)
	NotEqualSkip(t, 2, val, nil)
	EqualSkip(t, 2, val, expected)
}

func TestEqual(t *testing.T) {

	// error comes from your package/library
	err := errors.New("my error")
	NotEqual(t, err, nil)
	Equal(t, err.Error(), "my error")

	err = nil
	Equal(t, err, nil)

	fn := func() {
		panic("omg omg omg!")
	}

	PanicMatches(t, func() { fn() }, "omg omg omg!")
	PanicMatches(t, func() { panic("omg omg omg!") }, "omg omg omg!")

	// errs would have come from your package/library
	errs := map[string]string{}
	errs["Name"] = "User Name Invalid"
	errs["Email"] = "User Email Invalid"

	AssertCustomErrorHandler(t, errs, "Name", "User Name Invalid")
	AssertCustomErrorHandler(t, errs, "Email", "User Email Invalid")
}
```

How to Contribute
------
Make a PR.

I strongly encourage everyone whom creates a usefull custom assertion function to contribute them and
help make this package even better.

License
------
Distributed under MIT License, please see license file in code for more details.
