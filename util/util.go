/*
* Archon PSO Server
* Copyright (C) 2014 Andrew Rodman
*
* This program is free software: you can redistribute it and/or modify
* it under the terms of the GNU General Public License as published by
* the Free Software Foundation, either version 3 of the License, or
* (at your option) any later version.
*
* This program is distributed in the hope that it will be useful,
* but WITHOUT ANY WARRANTY; without even the implied warranty of
* MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
* GNU General Public License for more details.
*
* You should have received a copy of the GNU General Public License
* along with this program.  If not, see <http://www.gnu.org/licenses/>.
* ---------------------------------------------------------------------
*
* Functions and constants shared between server components.
 */
package util

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"unicode/utf16"
)

const displayWidth = 16

// Extract the packet length from the first two bytes of data.
func GetPacketSize(data []byte) (uint16, error) {
	if len(data) < 2 {
		return 0, errors.New("getSize(): data must be at least two bytes")
	}
	var size uint16
	reader := bytes.NewReader(data)
	binary.Read(reader, binary.LittleEndian, &size)
	return size, nil
}

func Utf8To16(src []uint8) []uint16 {
	if len(src)%2 != 0 {
		src = append(src, 0)
	}
	arr := make([]uint16, len(src)/2)
	fmt.Println("arrlen: ", len(arr))
	for i := 0; i < len(arr); i++ {
		fmt.Println("i:", i)
		arr[i] = uint16(src[i*2] | (src[(i*2)+1] << 8))
	}
	return arr
}

// Expands an array of UTF-16 elements to a slice of uint8 elements in
// little endian order. E.g: [0x1234] -> [0x34, 0x12]
func ExpandUtf16(src []uint16) []uint8 {
	expanded := make([]uint8, 2*len(src))
	for i, v := range src {
		idx := i * 2
		expanded[idx] = uint8(v)
		expanded[idx+1] = uint8((v >> 8) & 0xFF)
	}
	return expanded
}

// Convert a UTF-8 string to UTF-16 LE and return it as an array of bytes.
func ConvertToUtf16(str string) []byte {
	strRunes := bytes.Runes([]byte(str))
	return ExpandUtf16(utf16.Encode(strRunes))
}

// Returns a slice of b without the trailing 0s.
func StripPadding(b []byte) []byte {
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] != 0 {
			return b[:i+1]
		}
	}
	return b
}

// Sets the values of a slice of bytes (up to length) to 0.
func ZeroSlice(arr []byte, length int) {
	if arrLen := len(arr); arrLen < length {
		length = arrLen
	}
	for i := 0; i < length; i++ {
		arr[i] = 0
	}
}

// Serializes the fields of a struct to an array of bytes in the order in
// which the fields are declared. Calls panic() if data is not a struct or
// pointer to struct, or if there was an error writing a field.
func BytesFromStruct(data interface{}) ([]byte, int) {
	val := reflect.ValueOf(data)
	valKind := val.Kind()
	if valKind == reflect.Ptr {
		val = reflect.ValueOf(data).Elem()
		valKind = val.Kind()
	}

	if valKind != reflect.Struct {
		panic("BytesFromStruct(): data must of type struct " +
			"or ptr to struct, got: " + valKind.String())
	}

	bytes := new(bytes.Buffer)
	// It's possible to use binary.Write on val.Interface itself, but doing
	// so prevents this function from working with dynamically sized types.
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)

		var err error
		switch kind := field.Kind(); kind {
		case reflect.Struct, reflect.Ptr:
			b, _ := BytesFromStruct(field.Interface())
			err = binary.Write(bytes, binary.LittleEndian, b)
		default:
			err = binary.Write(bytes, binary.LittleEndian, field.Interface())
		}
		if err != nil {
			panic(err.Error())
		}
	}
	return bytes.Bytes(), bytes.Len()
}

// Populates the struct pointed to by targetStruct by reading in a stream of
// bytes and filling the values in sequential order.
func StructFromBytes(data []byte, targetStruct interface{}) {
	targetVal := reflect.ValueOf(targetStruct)
	if valKind := targetVal.Kind(); valKind != reflect.Ptr {
		panic("StructFromBytes(): targetStruct must be a " +
			"ptr to struct, got: " + valKind.String())
	}
	reader := bytes.NewReader(data)
	val := targetVal.Elem()
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)

		var err error
		switch field.Kind() {
		case reflect.Ptr:
			err = binary.Read(reader, binary.LittleEndian, field.Interface())
		default:
			err = binary.Read(reader, binary.LittleEndian, field.Addr().Interface())
		}
		if err != nil {
			panic(err.Error())
		}
	}
}

// Write one line of data to stdout.
func printPacketLine(data []uint8, length int, offset int) {
	fmt.Printf("(%04X) ", offset)
	// Print our bytes.
	for i, j := 0, 0; i < length; i++ {
		if j == 8 {
			// Visual aid - spacing between groups of 8 bytes.
			j = 0
			fmt.Print("  ")
		}
		fmt.Printf("%02x ", data[i])
		j++
	}
	// Fill in the gap if we don't have enough bytes to fill the line.
	for i := length; i < displayWidth; i++ {
		if i == 8 {
			fmt.Print("  ")
		}
		fmt.Print("   ")
	}
	fmt.Print("    ")
	// Display the print characters as-is, others as periods.
	for i := 0; i < length; i++ {
		c := data[i]
		if strconv.IsPrint(rune(c)) {
			fmt.Printf("%c", data[i])
		} else {
			fmt.Print(".")
		}
	}
	fmt.Println()
}

// Print the contents of a packet to stdout in two columns, one for bytes and
// the other for their ascii representation.
func PrintPayload(data []uint8, pktLen int) {
	for rem, offset := pktLen, 0; rem > 0; rem -= displayWidth {
		if rem < displayWidth {
			printPacketLine(data[(pktLen-rem):pktLen], rem, offset)
		} else {
			printPacketLine(data[offset:offset+displayWidth], displayWidth, offset)
		}
		offset += displayWidth
	}
}
