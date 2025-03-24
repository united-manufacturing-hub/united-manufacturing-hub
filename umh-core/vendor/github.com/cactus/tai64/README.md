tai64
=====

[![Build Status](https://github.com/cactus/tai64/workflows/unit-tests/badge.svg)](https://github.com/cactus/tai64/actions)
[![GoDoc](https://godoc.org/github.com/cactus/tai64?status.png)](https://godoc.org/github.com/cactus/tai64)
[![Go Report Card](https://goreportcard.com/badge/github.com/cactus/tai64)](https://goreportcard.com/report/github.com/cactus/tai64)
[![License](https://img.shields.io/github/license/cactus/tai64.svg)](https://github.com/cactus/tai64/blob/master/LICENSE.md)

## About

Formats and parses [TAI64 and TAI64N][1] timestamps.

## Usage

``` go
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/cactus/tai64"
)

func main() {
	t := time.Now()
	fmt.Println(t)

	s := tai64.FormatNano(t)
	fmt.Println(s)

	p, err := tai64.Parse(s)
	if err != nil {
		fmt.Println("Failed to decode time")
		os.Exit(1)
	}

    // tai64 times are in UTC
    fmt.Println(p)

    // time.Equal properly compares times with different locations.
	if t.Equal(p) {
		fmt.Println("equal")
	} else {
		fmt.Println("not equal")
	}
}
```

Output:

```
2016-05-25 13:44:01.281160355 -0700 PDT
@4000000057460eb510c22aa3
2016-05-25 20:44:01.281160355 +0000 UTC
equal
```

## License

Released under the [ISC license][2]. See `LICENSE.md` file for details.


[1]: https://cr.yp.to/libtai/tai64.html
[2]: https://choosealicense.com/licenses/isc/
