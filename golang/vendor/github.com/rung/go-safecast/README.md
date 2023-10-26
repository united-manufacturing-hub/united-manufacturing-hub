# go-safecast
Library for safe type conversion in Go

# What is this
The type of `int` equals `int64` on 64-bit machine in Go.  
When you convert `int`(`int64`) to `int32`, `int8` or `int6`, Your code could have Integer Overflow vulnerability.

In 2019, [Kubernetes](https://kubernetes.io/) had the vulnerability. and the vulnerability was found on [Security Audit Project](https://github.com/kubernetes/community/blob/master/sig-security/security-audit-2019/findings/Kubernetes%20Final%20Report.pdf) by Trail of Bits.

You can use this library to prevent the vulnerability creation.

**(This library is inspired by Kubernetes's Security Audit Report by Trail of Bits)**

# Usage

```go
import "github.com/rung/go-safecast"
```

## Convert int to int32 (instead of native int32() type conversion)
```go
	i := 2147483647
	i32, err := safecast.Int32(i) // convert int to int32 in a safe way
	if err != nil {
		return err
	}
```
This library also has `safecast.Int16` and `safecast.Int8`. You can use the functions in the same way as `safecast.Int32`

## Convert string to int32 (instead of strconv.Atoi())
```go
	s := "2147483647"
	i, err := safecast.Atoi32(s) // convert string to int32 in a safe way
	if err != nil {
		return err
	}
```
This library also has `safecast.Atoi16` and `safecast.Atoi8`. You can use the functions in the same way as `safecast.Atoi32`  

# What happens when overflows
## Range of each integer
|       | int32 (32bit signed integer)         | int16 (16bit signed integer) | int8 (8bit signed integer) | 
| :---: | :----------------------------------: | :--------------------------: | :------------------------: | 
| Range | From -2,147,483,648 to 2,147,483,647 | From -32,768 to 32,767       | From -128 to 127           | 

## When using native int32(), the code causes overflows
<img src="img/native-int32.png" width="700px">  

Link: [Go Playground](https://play.golang.org/p/tyATM4dL33x)

---

## When using safecast.Int32() on this library, your code is safe
<img src="img/safecast-int32.png" width="700px">  
This library can detect integer overflow. so you can convert integer in a safe way.  

Link: [Go Playground](https://play.golang.org/p/1xeeyt-feLI)

# License
[MIT License](LICENSE)
