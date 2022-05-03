// Copyright Banrai LLC. All rights reserved. Use of this source code is
// governed by the license that can be found in the LICENSE file.

// Package scanner provides functions for reading barcode scans from
// usb-connected barcode scanner devices as if they were keyboards, i.e.,
// by using the corresponding '/dev/input/event' device, inspired by this
// post on linuxquestions.org:
//
// http://www.linuxquestions.org/questions/programming-9/read-from-a-usb-barcode-scanner-that-simulates-a-keyboard-495358/#post2767643
//
// Also found important Go-specific information by reviewing the code from
// this repo on github:
//
// https://github.com/gvalkov/golang-evdev

//https://github.com/Banrai/PiScan/blob/master/LICENSE

package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/gvalkov/golang-evdev"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"os"
	"strconv"
	"syscall"
	"time"
	"unsafe"
)

const (
	EVENT_CAPTURES = 16
)

// InputEvent is a Go implementation of the native linux device
// input_event struct, as described in the kernel documentation
// (https://www.kernel.org/doc/Documentation/input/input.txt),
// with a big assist from https://github.com/gvalkov/golang-evdev
type InputEvent struct {
	Time  syscall.Timeval
	Type  uint16
	Code  uint16
	Value int32
}

var EVENT_SIZE = int(unsafe.Sizeof(InputEvent{}))

type KeyValues struct {
	normal rune
	shift  rune
}

// KEYMAP maps evdev to unicode values
var KEYMAP = map[int]KeyValues{

	evdev.KEY_0: {
		normal: '0',
		shift:  ')',
	},
	evdev.KEY_1: {
		normal: '1',
		shift:  '!',
	},
	evdev.KEY_2: {
		normal: '2',
		shift:  '@',
	},
	evdev.KEY_3: {
		normal: '3',
		shift:  '#',
	},
	evdev.KEY_4: {
		normal: '4',
		shift:  '$',
	},
	evdev.KEY_5: {
		normal: '5',
		shift:  '%',
	},
	evdev.KEY_6: {
		normal: '6',
		shift:  '^',
	},
	evdev.KEY_7: {
		normal: '7',
		shift:  '&',
	},
	evdev.KEY_8: {
		normal: '8',
		shift:  '*',
	},
	evdev.KEY_9: {
		normal: '9',
		shift:  '(',
	},
	evdev.KEY_A: {
		normal: 'a',
		shift:  'A',
	},
	evdev.KEY_APOSTROPHE: {
		normal: '\'',
		shift:  '"',
	},
	evdev.KEY_B: {
		normal: 'b',
		shift:  'B',
	},
	evdev.KEY_BACKSLASH: {
		normal: '\\',
		shift:  '|',
	},
	evdev.KEY_BACKSPACE: {
		normal: '\b',
		shift:  '-',
	},
	evdev.KEY_C: {
		normal: 'c',
		shift:  'C',
	},
	evdev.KEY_COMMA: {
		normal: ',',
		shift:  '<',
	},
	evdev.KEY_D: {
		normal: 'd',
		shift:  'D',
	},
	evdev.KEY_DOLLAR: {
		normal: '$',
		shift:  '-',
	},
	evdev.KEY_DOT: {
		normal: '.',
		shift:  '>',
	},
	evdev.KEY_E: {
		normal: 'e',
		shift:  'E',
	},
	evdev.KEY_ENTER: {
		normal: '\n',
		shift:  '-',
	},
	evdev.KEY_EQUAL: {
		normal: '=',
		shift:  '+',
	},
	evdev.KEY_EURO: {
		normal: '\u20ac',
		shift:  '-',
	},
	evdev.KEY_F: {
		normal: 'f',
		shift:  'F',
	},
	evdev.KEY_G: {
		normal: 'g',
		shift:  'G',
	},
	evdev.KEY_GRAVE: {
		normal: '`',
		shift:  '~',
	},
	evdev.KEY_H: {
		normal: 'h',
		shift:  'H',
	},
	evdev.KEY_I: {
		normal: 'i',
		shift:  'I',
	},
	evdev.KEY_J: {
		normal: 'j',
		shift:  'J',
	},
	evdev.KEY_K: {
		normal: 'k',
		shift:  'K',
	},
	evdev.KEY_KP0: {
		normal: '0',
		shift:  ')',
	},
	evdev.KEY_KP1: {
		normal: '1',
		shift:  '!',
	},
	evdev.KEY_KP2: {
		normal: '2',
		shift:  '@',
	},
	evdev.KEY_KP3: {
		normal: '3',
		shift:  '#',
	},
	evdev.KEY_KP4: {
		normal: '4',
		shift:  '$',
	},
	evdev.KEY_KP5: {
		normal: '5',
		shift:  '%',
	},
	evdev.KEY_KP6: {
		normal: '6',
		shift:  '^',
	},
	evdev.KEY_KP7: {
		normal: '7',
		shift:  '&',
	},
	evdev.KEY_KP8: {
		normal: '8',
		shift:  '*',
	},
	evdev.KEY_KP9: {
		normal: '9',
		shift:  '(',
	},
	evdev.KEY_KPASTERISK: {
		normal: '*',
		shift:  '-',
	},
	evdev.KEY_KPCOMMA: {
		normal: ',',
		shift:  '<',
	},
	evdev.KEY_KPDOT: {
		normal: '.',
		shift:  '>',
	},
	evdev.KEY_KPENTER: {
		normal: '\n',
		shift:  '-',
	},
	evdev.KEY_KPEQUAL: {
		normal: '=',
		shift:  '+',
	},
	evdev.KEY_KPMINUS: {
		normal: '-',
		shift:  '_',
	},
	evdev.KEY_KPPLUS: {
		normal: '+',
		shift:  '-',
	},
	evdev.KEY_KPSLASH: {
		normal: '/',
		shift:  '?',
	},
	evdev.KEY_L: {
		normal: 'l',
		shift:  'L',
	},
	evdev.KEY_LEFTBRACE: {
		normal: '[',
		shift:  '{',
	},
	evdev.KEY_LINEFEED: {
		normal: '\n',
		shift:  '-',
	},
	evdev.KEY_M: {
		normal: 'm',
		shift:  'M',
	},
	evdev.KEY_MINUS: {
		normal: '-',
		shift:  '_',
	},
	evdev.KEY_N: {
		normal: 'n',
		shift:  'N',
	},
	evdev.KEY_NUMERIC_0: {
		normal: '0',
		shift:  ')',
	},
	evdev.KEY_NUMERIC_1: {
		normal: '1',
		shift:  '!',
	},
	evdev.KEY_NUMERIC_2: {
		normal: '2',
		shift:  '@',
	},
	evdev.KEY_NUMERIC_3: {
		normal: '3',
		shift:  '#',
	},
	evdev.KEY_NUMERIC_4: {
		normal: '4',
		shift:  '$',
	},
	evdev.KEY_NUMERIC_5: {
		normal: '5',
		shift:  '%',
	},
	evdev.KEY_NUMERIC_6: {
		normal: '6',
		shift:  '^',
	},
	evdev.KEY_NUMERIC_7: {
		normal: '7',
		shift:  '&',
	},
	evdev.KEY_NUMERIC_8: {
		normal: '8',
		shift:  '*',
	},
	evdev.KEY_NUMERIC_9: {
		normal: '9',
		shift:  '(',
	},
	evdev.KEY_NUMERIC_A: {
		normal: 'a',
		shift:  'A',
	},
	evdev.KEY_NUMERIC_B: {
		normal: 'b',
		shift:  'B',
	},
	evdev.KEY_NUMERIC_C: {
		normal: 'c',
		shift:  'C',
	},
	evdev.KEY_NUMERIC_D: {
		normal: 'd',
		shift:  'D',
	},
	evdev.KEY_NUMERIC_POUND: {
		normal: '\u00a3',
		shift:  '-',
	},
	evdev.KEY_NUMERIC_STAR: {
		normal: '*',
		shift:  '-',
	},
	evdev.KEY_O: {
		normal: 'o',
		shift:  'O',
	},
	evdev.KEY_P: {
		normal: 'p',
		shift:  'P',
	},
	evdev.KEY_Q: {
		normal: 'q',
		shift:  'Q',
	},
	evdev.KEY_R: {
		normal: 'r',
		shift:  'R',
	},
	evdev.KEY_RIGHTBRACE: {
		normal: ']',
		shift:  '}',
	},
	evdev.KEY_S: {
		normal: 's',
		shift:  'S',
	},
	evdev.KEY_SEMICOLON: {
		normal: ';',
		shift:  ':',
	},
	evdev.KEY_SLASH: {
		normal: '/',
		shift:  '?',
	},
	evdev.KEY_SPACE: {
		normal: ' ',
		shift:  ' ',
	},
	evdev.KEY_T: {
		normal: 't',
		shift:  'T',
	},
	evdev.KEY_TAB: {
		normal: '\t',
		shift:  '-',
	},
	evdev.KEY_U: {
		normal: 'u',
		shift:  'U',
	},
	evdev.KEY_V: {
		normal: 'v',
		shift:  'V',
	},
	evdev.KEY_W: {
		normal: 'w',
		shift:  'W',
	},
	evdev.KEY_X: {
		normal: 'x',
		shift:  'X',
	},
	evdev.KEY_Y: {
		normal: 'y',
		shift:  'Y',
	},
	evdev.KEY_Z: {
		normal: 'z',
		shift:  'Z',
	},
}

var shiftKey = false

// lookupKeyCode finds the corresponding string for the given hex byte,
// returning "-" as the default if not found
func lookupKeyCode(b byte) (rune rune, validKeyCode bool, foundInKeyMap bool, isModifier bool, altPressed bool) {
	zap.S().Debugf("Looking for %v (shift: %v)", evdev.KEY[int(b)], shiftKey)
	val, isLinuxKeyCode := evdev.KEY[int(b)]
	if !isLinuxKeyCode {
		zap.S().Warnf("Key code %x not found in linux key code map", b)
		return '-', false, false, false, false
	}
	v, ok := KEYMAP[int(b)]
	if !ok {
		switch int(b) {
		case evdev.KEY_CAPSLOCK, evdev.KEY_RIGHTSHIFT, evdev.KEY_LEFTSHIFT:
			shiftKey = true
			return '-', false, false, true, false
		case evdev.KEY_NUMLOCK:
			// Don't really know what i should do this one !
			return '-', false, false, true, false
		case evdev.KEY_LEFTALT:
			zap.S().Debugf("Pressing ALT")
			return '-', false, false, true, true

		}

		zap.S().Warnf("Keycode %s not found in KEYMAP", val)
		return '-', true, false, false, false
	}

	zap.S().Debugf("Found: %s (%s)", string(v.normal), string(v.shift))

	if shiftKey {
		shiftKey = false
		return v.shift, true, true, false, false
	}
	return v.normal, true, true, false, false
}

func resetKeyModifiers() {
	shiftKey = false
}

// read takes the open scanner device pointer and returns a list of
// InputEvent captures, corresponding to input (scan) events
func read(dev *os.File) ([]InputEvent, error) {
	events := make([]InputEvent, EVENT_CAPTURES)
	buffer := make([]byte, EVENT_SIZE*EVENT_CAPTURES)
	_, err := dev.Read(buffer)
	if err != nil {
		fmt.Printf("dev.Read failed with error: %s\n", err)
		return events, err
	}
	b := bytes.NewBuffer(buffer)
	err = binary.Read(b, binary.LittleEndian, &events)
	if err != nil {
		fmt.Printf("binary.Read failed with error: %s\n", err)
		return events, err
	}
	// remove trailing structures
	for i := range events {
		if events[i].Time.Sec == 0 {
			events = append(events[:i])
			break
		}
	}
	return events, err
}

var altPressedState = false
var altBuffer = make([]byte, 0)

// DecodeEvents iterates through the list of InputEvents and decodes
// the barcode data into a string, along with a boolean to indicate if this
// particular input sequence is done
func decodeEvents(events []InputEvent) (string, bool) {
	var buffer bytes.Buffer
	for i := range events {
		if events[i].Type == 1 && events[i].Value == 1 {
			if events[i].Code == 28 {
				// carriage return detected: the barcode sequence ends here
				return buffer.String(), true
			} else {
				if events[i].Code != 0 {
					// this is barcode data we want to capture
					keyCode, _, _, isModifier, altPressed := lookupKeyCode(byte(events[i].Code))

					if !isModifier {
						if altPressedState {
							altBuffer = append(altBuffer, byte(keyCode))
							if len(altBuffer) == 4 {
								bufNum, err := strconv.Atoi(string(altBuffer))
								if err != nil {
									zap.S().Warnf("Error converting alt buffer to int: %s", err)
								}
								//goland:noinspection ALL
								buffer.WriteString(string(bufNum))
								altBuffer = nil
								altPressedState = false
							}
						} else {
							buffer.WriteString(string(keyCode))
						}
					} else {
						if altPressed {
							altPressedState = true
						}
					}
				}
			}
		}
	}
	// return what has been collected so far,
	// even though the barcode is not yet complete
	return buffer.String(), false
}

// ScanForever takes a linux input device string pointing to the scanner
// to read from, invokes the given function on the resulting barcode string
// when complete, or the errfn on error, then goes back to read/scan again
func ScanForever(device *evdev.InputDevice, fn func(string), errFn func(error)) {

	var scanBuffer bytes.Buffer
	for {
		scanEvents, scanErr := read(device.File)
		if scanErr != nil {
			// invoke the function which handles scanner errors
			errFn(scanErr)
			time.Sleep(internal.OneSecond)
		}
		scannedData, endOfScan := decodeEvents(scanEvents)
		if endOfScan {
			// invoke the function which handles the scan result
			fn(scanBuffer.String())
			scanBuffer.Reset() // clear the buffer and start again
			altPressedState = false
			altBuffer = nil
			resetKeyModifiers()
		} else {
			scanBuffer.WriteString(scannedData)
		}
	}
}
