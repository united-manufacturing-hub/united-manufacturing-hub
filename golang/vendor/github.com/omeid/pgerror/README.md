# pgerror [![GoDoc](https://img.shields.io/badge/godoc-reference-blue.svg?style=flat-square)](https://godoc.org/github.com/omeid/pgerror) [![Build Status](https://travis-ci.org/omeid/pgerror.svg?branch=master)](https://travis-ci.org/omeid/pgerror) [![Go Report Card](https://goreportcard.com/badge/github.com/omeid/pgerror)](https://goreportcard.com/report/github.com/omeid/pgerror)

pgerror is a collection of helper functions to use with github.com/lib/pq Postgresql Database driver for Go Programming language.

```go

// example use:
_, err = stmt.Exec(SomeInsertStateMent, params...)
if err != nil {
  if e := pgerror.UniqueViolation(err); e != nil {
  // you can use e here to check the fields et al
    return SomeThingAlreadyExists
  }

  return err // other cases.
}

```

### LICENSE

[MIT](LICENSE).
