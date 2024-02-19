package parse

/*
Copyright 2023 UMH Systems GmbH

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

import (
	"errors"
	inf "gopkg.in/inf.v0"
	"math/big"
	"strconv"
	"strings"
)

type suffix string

// Format lists the three possible formattings of a quantity.
type Format string

// Scale is used for getting and setting the base-10 scaled value.
// Base-2 scales are omitted for mathematical simplicity.
// See Quantity.ScaledValue for more details.
type Scale int32

const (
	// splitREString is used to separate a number from its suffix; as such,
	// this is overly permissive, but that's OK-- it will be checked later.
	splitREString   = "^([+-]?[0-9.]+)([eEinumkKMGTP]*[-+]?[0-9]*)$"
	DecimalExponent = Format("DecimalExponent") // e.g., 12e6
	BinarySI        = Format("BinarySI")        // e.g., 12Mi (12 * 2^20)
	DecimalSI       = Format("DecimalSI")       // e.g., 12M  (12 * 10^6)
)

const (
	Nano Scale = -9
)

const (
	mostNegative = -(mostPositive + 1)
	mostPositive = 1<<63 - 1
)

var (
	// Errors that could happen while parsing a string.
	ErrFormatWrong = errors.New("quantities must match the regular expression '" + splitREString + "'")
	ErrNumeric     = errors.New("unable to parse numeric part of quantity")
	ErrSuffix      = errors.New("unable to parse quantity's suffix")
	ErrOverflow    = errors.New("unable to parse quantity due to overflow")
	ErrScale       = errors.New("unable to parse quantity due to scale")
)

var (
	// Commonly needed big.Int values-- treat as read only!
	bigOne = big.NewInt(1)
)

// Quantity turns str into an int, or returns an error.
func Quantity(str string) (int, error) {
	if len(str) == 0 {
		return 0, ErrFormatWrong
	}
	if str == "0" {
		return 0, nil
	}

	positive, value, num, denom, suf, err := parseQuantityString(str)
	if err != nil {
		return 0, err
	}

	base, exponent, format, ok := interpret(suffix(suf))
	if !ok {
		return 0, ErrSuffix
	}

	precision := int32(0)
	scale := int32(0)
	mantissa := int64(1)
	switch format {
	case DecimalExponent, DecimalSI:
		scale = exponent
		precision = 18 - int32(len(num)+len(denom))
	case BinarySI:
		scale = 0
		switch {
		case exponent >= 0 && len(denom) == 0:
			// only handle positive binary numbers with the fast path
			mantissa = mantissa << uint64(exponent)
			// 1Mi (2^20) has ~6 digits of decimal precision, so exponent*3/10 -1 is roughly the precision
			precision = 15 - int32(len(num)) - int32(float32(exponent)*3/10) - 1
		default:
			precision = -1
		}
	}

	if precision >= 0 {
		// if we have a denominator, shift the entire value to the left by the number of places in the
		// denominator
		scale -= int32(len(denom))
		if scale >= int32(Nano) {
			shifted := num + denom

			var value int64
			value, err := strconv.ParseInt(shifted, 10, 64)
			if err != nil {
				return 0, ErrNumeric
			}
			result := value * mantissa
			if !positive {
				result = -result
			}
			// if the number is in canonical form, reuse the string
			switch format {
			case BinarySI:
				if exponent%10 == 0 && (value&0x07 != 0) {
					return AsInt(int(result), Scale(scale))
				}
			default:
				if scale%3 == 0 && !strings.HasSuffix(shifted, "000") && shifted[0] != '0' {
					return AsInt(int(result), Scale(scale))
				}
			}
			return AsInt(int(result), Scale(scale))
		}
	}

	amount := new(inf.Dec)
	if _, ok = amount.SetString(value); !ok {
		return 0, ErrNumeric
	}

	// So that no one but us has to think about suffixes, remove it.
	if base == 10 {
		amount.SetScale(amount.Scale() + inf.Scale(-scale))
	} else if base == 2 {
		// numericSuffix = 2 ** exponent
		numericSuffix := big.NewInt(1).Lsh(bigOne, uint(exponent))
		ub := amount.UnscaledBig()
		amount.SetUnscaledBig(ub.Mul(ub, numericSuffix))
	}

	// Cap at min/max bounds.
	sign := amount.Sign()
	if sign == -1 {
		amount.Neg(amount)
	}

	// This rounds non-zero values up to the minimum representable value, under the theory that
	// if you want some resources, you should get some resources, even if you asked for way too small
	// of an amount.  Arguably, this should be inf.RoundHalfUp (normal rounding), but that would have
	// the side effect of rounding values < .5n to zero.
	var v int64
	if v, ok = amount.Unscaled(); v != int64(0) || !ok {
		amount.Round(amount, inf.Scale(-Nano), inf.RoundUp)
	}

	v = amount.UnscaledBig().Int64()
	return AsInt(int(v), Scale(amount.Scale()))
}

// parseQuantityString is a fast scanner for quantity values.
func parseQuantityString(str string) (positive bool, value, num, denom, suffix string, err error) {
	positive = true
	pos := 0
	end := len(str)

	// handle leading sign
	if pos < end {
		switch str[0] {
		case '-':
			positive = false
			pos++
		case '+':
			pos++
		}
	}

	// strip leading zeros
Zeroes:
	for i := pos; ; i++ {
		if i >= end {
			num = "0"
			value = num
			return
		}
		switch str[i] {
		case '0':
			pos++
		default:
			break Zeroes
		}
	}

	// extract the numerator
Num:
	for i := pos; ; i++ {
		if i >= end {
			num = str[pos:end]
			value = str[0:end]
			return
		}
		switch str[i] {
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		default:
			num = str[pos:i]
			pos = i
			break Num
		}
	}

	// if we stripped all numerator positions, always return 0
	if len(num) == 0 {
		num = "0"
	}

	// handle a denominator
	if pos < end && str[pos] == '.' {
		pos++
	Denom:
		for i := pos; ; i++ {
			if i >= end {
				denom = str[pos:end]
				value = str[0:end]
				return
			}
			switch str[i] {
			case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			default:
				denom = str[pos:i]
				pos = i
				break Denom
			}
		}
		// TODO: we currently allow 1.G, but we may not want to in the future.
		// if len(denom) == 0 {
		// 	err = ErrFormatWrong
		// 	return
		// }
	}
	value = str[0:pos]

	// grab the elements of the suffix
	suffixStart := pos
	for i := pos; ; i++ {
		if i >= end {
			suffix = str[suffixStart:end]
			return
		}
		if !strings.ContainsAny(str[i:i+1], "eEinumkKMGTP") {
			pos = i
			break
		}
	}
	if pos < end {
		switch str[pos] {
		case '-', '+':
			pos++
		}
	}
Suffix:
	for i := pos; ; i++ {
		if i >= end {
			suffix = str[suffixStart:end]
			return
		}
		switch str[i] {
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		default:
			break Suffix
		}
	}
	// we encountered a non decimal in the Suffix loop, but the last character
	// was not a valid exponent
	err = ErrFormatWrong
	return
}

func interpret(s suffix) (base, exponent int32, format Format, ok bool) {
	switch s {
	case "":
		return 10, 0, DecimalSI, true
	case "n":
		return 10, -9, DecimalSI, true
	case "u":
		return 10, -6, DecimalSI, true
	case "m":
		return 10, -3, DecimalSI, true
	case "k":
		return 10, 3, DecimalSI, true
	case "M":
		return 10, 6, DecimalSI, true
	case "G":
		return 10, 9, DecimalSI, true
	case "Ki":
		return 2, 10, BinarySI, true
	case "Mi":
		return 2, 20, BinarySI, true
	case "Gi":
		return 2, 30, BinarySI, true
	}
	return 0, 0, BinarySI, false
}

// AsInt returns the current amount as an int64 at scale 0, or false if the value cannot be
// represented in an int64 OR would result in a loss of precision. This method is intended as
// an optimization to avoid calling AsDec.
func AsInt(value int, scale Scale) (int, error) {
	if scale == 0 {
		return value, nil
	}
	if scale < 0 {
		// TODO: attempt to reduce factors, although it is assumed that factors are reduced prior
		// to the int64Amount being created.
		return 0, ErrScale
	}
	return positiveScaleInt(value, scale)
}

// positiveScaleInt multiplies base by 10^scale, returning false if the
// value overflows. Passing a negative scale is undefined.
func positiveScaleInt(base int, scale Scale) (int, error) {
	switch scale {
	case 0:
		return base, nil
	case 1:
		return intMultiplyScale(base, 10)
	case 2:
		return intMultiplyScale(base, 100)
	case 3:
		return intMultiplyScale(base, 1000)
	case 6:
		return intMultiplyScale(base, 1000000)
	case 9:
		return intMultiplyScale(base, 1000000000)
	default:
		value := base
		err := error(nil)
		for i := Scale(0); i < scale; i++ {
			value, err = intMultiplyScale(value, 10)
			if err != nil {
				return 0, err
			}
		}
		return value, nil
	}
}

// intMultiplyScale returns a*b, assuming b is greater than one, or false if that would overflow or underflow int64.
// Use when b is known to be greater than one.
func intMultiplyScale(a int, b int) (int, error) {
	if a == 0 || a == 1 {
		return a * b, nil
	}
	if a == mostNegative && b != 1 {
		return 0, ErrOverflow
	}
	c := a * b
	return c, nil
}
