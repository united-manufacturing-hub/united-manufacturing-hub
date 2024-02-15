# base64

[![build-img]][build-url]
[![pkg-img]][pkg-url]
[![reportcard-img]][reportcard-url]
[![coverage-img]][coverage-url]

Faster base64 encoding for Go, based on [Turbo-Base64](https://github.com/powturbo/Turbo-Base64).

## Features

* Drop-in replacement of `encoding/base64`.
  * _except for error messages and ignoring `\r` and `\n` in decoder._
* up to 3 times faster than `encoding/base64`.
* Dependency-free.

## Install

Go version 1.16+

```
go get github.com/cristalhq/base64
```

## How to use

Replace import statement from `encoding/base64` to `github.com/cristalhq/base64`

```diff
-import "encoding/base64"
+import "github.com/cristalhq/base64"
```

# Benchmarks

go1.17 linux/amd64, Intel i7-7700

```
std/Encode           685.3 ns/op      0 B/op   0 allocs/op
std/EncodeToString   951.8 ns/op   2048 B/op   2 allocs/op
std/Decode           803.9 ns/op      0 B/op   0 allocs/op
std/DecodeString     1061 ns/op    1792 B/op   2 allocs/op

own/Encode           217.8 ns/op      0 B/op   0 allocs/op
own/EncodeToString   353.2 ns/op   1024 B/op   1 allocs/op
own/Decode           426.0 ns/op      0 B/op   0 allocs/op
own/DecodeString     598.7 ns/op    768 B/op   1 allocs/op
```

go1.17 darwin/arm64, Apple M1

```
std/Encode           413.0 ns/op       0 B/op  0 allocs/op
std/EncodeToString   608.3 ns/op    2048 B/op  2 allocs/op
std/Decode           372.5 ns/op       0 B/op  0 allocs/op
std/DecodeString     570.2 ns/op    1792 B/op  2 allocs/op

own/Encode           146.7 ns/op       0 B/op  0 allocs/op
own/EncodeToString   246.4 ns/op    1024 B/op  1 allocs/op
own/Decode           222.8 ns/op       0 B/op  0 allocs/op
own/DecodeString     303.1 ns/op     768 B/op  1 allocs/op
```

# [Fuzzing](fuzz)

## Documentation

See [these docs][pkg-url].

## License

[MIT License](LICENSE).

[build-img]: https://github.com/cristalhq/base64/workflows/build/badge.svg
[build-url]: https://github.com/cristalhq/base64/actions
[pkg-img]: https://pkg.go.dev/badge/cristalhq/base64
[pkg-url]: https://pkg.go.dev/github.com/cristalhq/base64
[reportcard-img]: https://goreportcard.com/badge/cristalhq/base64
[reportcard-url]: https://goreportcard.com/report/cristalhq/base64
[coverage-img]: https://codecov.io/gh/cristalhq/base64/branch/master/graph/badge.svg
[coverage-url]: https://codecov.io/gh/cristalhq/base64
