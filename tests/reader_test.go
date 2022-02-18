package tests

import (
	"bytes"
	"math/rand"
	"testing"

	"gopkg.in/yaml.v3/reader"
)

type pairRange struct {
	bounds [][2]byte
	valid  bool
}

var utf8Ranges = []pairRange{
	{[][2]byte{{0b00000000, 0b01111111}}, true},  // 0xxxxxxx
	{[][2]byte{{0b10000000, 0b11111111}}, false}, // 1xxxxxxx

	{[][2]byte{{0b10000000, 0b10111111}, {0b00000000, 0b11111111}}, false}, // ___xxxxx xxxxxxxx, with 0 < ___ < 110
	{[][2]byte{{0b11100000, 0b11111111}, {0b00000000, 0b11111111}}, false}, // ___xxxxx xxxxxxxx, with ___ > 110
	{[][2]byte{{0b10000000, 0b11111111}, {0b00000000, 0b01111111}}, false}, // 1xxxxxxx __xxxxxx, with __ < 10
	{[][2]byte{{0b10000000, 0b11111111}, {0b11000000, 0b11111111}}, false}, // 1xxxxxxx __xxxxxx, with __ > 10
	{[][2]byte{{0b11000000, 0b11000001}, {0b10000000, 0b10111111}}, false}, // 1100000x 10xxxxxx
	{[][2]byte{{0b11000010, 0b11011111}, {0b10000000, 0b10111111}}, true},  // 110bbbbx 10xxxxxx (other)

	{[][2]byte{{0b10000000, 0b11011111}, {0b00000000, 0b11111111}, {0b00000000, 0b11111111}}, false}, // ____xxxx xxxxxxxx xxxxxxxx, with 0 < ____ < 1110
	{[][2]byte{{0b11110000, 0b11111111}, {0b00000000, 0b11111111}, {0b00000000, 0b11111111}}, false}, // ____xxxx xxxxxxxx xxxxxxxx, with ____ > 1110
	{[][2]byte{{0b10000000, 0b11111111}, {0b00000000, 0b01111111}, {0b00000000, 0b11111111}}, false}, // 1xxxxxxx __xxxxxx xxxxxxxx, with __ < 10
	{[][2]byte{{0b10000000, 0b11111111}, {0b11000000, 0b11111111}, {0b00000000, 0b11111111}}, false}, // 1xxxxxxx __xxxxxx xxxxxxxx, with __ > 10
	{[][2]byte{{0b10000000, 0b11111111}, {0b00000000, 0b11111111}, {0b00000000, 0b01111111}}, false}, // 1xxxxxxx xxxxxxxx __xxxxxx, with __ < 10
	{[][2]byte{{0b10000000, 0b11111111}, {0b00000000, 0b11111111}, {0b11000000, 0b11111111}}, false}, // 1xxxxxxx xxxxxxxx __xxxxxx, with __ > 10
	{[][2]byte{{0b11100000, 0b11100000}, {0b10000000, 0b10011111}, {0b10000000, 0b10111111}}, false}, // 11100000 100xxxxx 10xxxxxx
	{[][2]byte{{0b11100000, 0b11100000}, {0b10100000, 0b10111111}, {0b10000000, 0b10111111}}, true},  // 11100000 101xxxxx 10xxxxxx
	{[][2]byte{{0b11100001, 0b11101100}, {0b10000000, 0b10111111}, {0b10000000, 0b10111111}}, true},  // 1110bbbb 10xxxxxx 10xxxxxx (other)
	{[][2]byte{{0b11101101, 0b11101101}, {0b10000000, 0b10011111}, {0b10000000, 0b10111111}}, true},  // 11101101 100xxxxx 10xxxxxx
	{[][2]byte{{0b11101101, 0b11101101}, {0b10100000, 0b10111111}, {0b10000000, 0b10111111}}, false}, // 11101101 101xxxxx 10xxxxxx - !(value >= 0xD800 && value <= 0xDFFF)
	{[][2]byte{{0b11101110, 0b11101111}, {0b10000000, 0b10111111}, {0b10000000, 0b10111111}}, true},  // 1110bbbb 10xxxxxx 10xxxxxx (other)

	{[][2]byte{{0b10000000, 0b11101111}, {0b00000000, 0b11111111}, {0b00000000, 0b11111111}, {0b00000000, 0b11111111}}, false}, // _____xxx xxxxxxxx xxxxxxxx xxxxxxxx, with 0 < _____ < 11110
	{[][2]byte{{0b11111000, 0b11111111}, {0b00000000, 0b11111111}, {0b00000000, 0b11111111}, {0b00000000, 0b11111111}}, false}, // _____xxx xxxxxxxx xxxxxxxx xxxxxxxx, with _____ > 11110
	{[][2]byte{{0b10000000, 0b11111111}, {0b00000000, 0b01111111}, {0b00000000, 0b11111111}, {0b00000000, 0b11111111}}, false}, // 1xxxxxxx __xxxxxx xxxxxxxx xxxxxxxx, with __ < 10
	{[][2]byte{{0b10000000, 0b11111111}, {0b11000000, 0b11111111}, {0b00000000, 0b11111111}, {0b00000000, 0b11111111}}, false}, // 1xxxxxxx __xxxxxx xxxxxxxx xxxxxxxx, with __ > 10
	{[][2]byte{{0b10000000, 0b11111111}, {0b00000000, 0b11111111}, {0b00000000, 0b01111111}, {0b00000000, 0b11111111}}, false}, // 1xxxxxxx xxxxxxxx __xxxxxx xxxxxxxx, with __ < 10
	{[][2]byte{{0b10000000, 0b11111111}, {0b00000000, 0b11111111}, {0b11000000, 0b11111111}, {0b00000000, 0b11111111}}, false}, // 1xxxxxxx xxxxxxxx __xxxxxx xxxxxxxx, with __ > 10
	{[][2]byte{{0b10000000, 0b11111111}, {0b00000000, 0b11111111}, {0b00000000, 0b11111111}, {0b00000000, 0b01111111}}, false}, // 1xxxxxxx xxxxxxxx xxxxxxxx __xxxxxx, with __ < 10
	{[][2]byte{{0b10000000, 0b11111111}, {0b00000000, 0b11111111}, {0b00000000, 0b11111111}, {0b11000000, 0b11111111}}, false}, // 1xxxxxxx xxxxxxxx xxxxxxxx __xxxxxx, with __ > 10
	{[][2]byte{{0b11110000, 0b11110000}, {0b10000000, 0b10001111}, {0b10000000, 0b10111111}, {0b10000000, 0b10111111}}, false}, // 11110000 1000xxxx 10xxxxxx 10xxxxxx
	{[][2]byte{{0b11110000, 0b11110000}, {0b10010000, 0b10111111}, {0b10000000, 0b10111111}, {0b10000000, 0b10111111}}, true},  // 11110000 10bbxxxx 10xxxxxx 10xxxxxx (other)
	{[][2]byte{{0b11110001, 0b11110011}, {0b10000000, 0b10111111}, {0b10000000, 0b10111111}, {0b10000000, 0b10111111}}, true},  // 111100bb 10xxxxxx 10xxxxxx 10xxxxxx (other)
	{[][2]byte{{0b11110100, 0b11110100}, {0b10000000, 0b10001111}, {0b10000000, 0b10111111}, {0b10000000, 0b10111111}}, true},  // 11110100 1000xxxx 10xxxxxx 10xxxxxx
	{[][2]byte{{0b11110100, 0b11110100}, {0b10010000, 0b10111111}, {0b10000000, 0b10111111}, {0b10000000, 0b10111111}}, false}, // 11110100 10bbxxxx 10xxxxxx 10xxxxxx - value < 0x10FFFF
	{[][2]byte{{0b11110101, 0b11110111}, {0b10000000, 0b10111111}, {0b10000000, 0b10111111}, {0b10000000, 0b10111111}}, false}, // 111101bb 10xxxxxx 10xxxxxx 10xxxxxx - value < 0x10FFFF
}

func validAndInvalidRanges() (validRanges [][][2]byte, invalidRanges [][][2]byte) {
	for _, r := range utf8Ranges {
		if r.valid {
			validRanges = append(validRanges, r.bounds)
		} else {
			invalidRanges = append(invalidRanges, r.bounds)
		}
	}
	return validRanges, invalidRanges
}

func generateAllBoundCombinations(bounds [][2]byte) [][]byte {
	nr_limits := len(bounds)
	combinations := [][]byte{}
	for i := 0; i < 1<<nr_limits; i++ {
		combination := []byte{}
		for j := 0; j < nr_limits; j++ {
			range_index := (i >> j) & 1
			combination = append(combination, bounds[j][range_index])
		}
		combinations = append(combinations, combination)
	}
	return combinations
}

func generateRandomCombination(bounds [][2]byte) []byte {
	nr_limits := len(bounds)
	combination := []byte{}
	for j := 0; j < nr_limits; j++ {
		low, high := int(bounds[j][0]), int(bounds[j][1])
		combination = append(combination, byte(rand.Intn(high-low)+low))
	}
	return combination
}

func checkResult(f *testing.T, testValue []byte, valid bool, result []byte, err error, n int) {
	if err != nil && valid {
		f.Errorf("value 0x%x: %v", testValue, err)
	} else if err == nil && !valid {
		f.Errorf("value 0x%x: Expected an error", testValue)
	} else if err == nil && n != len(testValue) {
		f.Errorf("value 0x%x: Expected length %d, got %d", testValue, len(testValue), n)
	} else if err != nil && n != 0 {
		f.Errorf("value 0x%x: Expected length 0, got %d", testValue, n)
	}
}

func TestUtf8RangesSingleValueFromBuffer(f *testing.T) {
	for _, r := range utf8Ranges {
		combinations := generateAllBoundCombinations(r.bounds)

		for _, testValue := range combinations {
			decoder := reader.NewUtfDecoderForBuffer(testValue)
			decoder.Encoding = reader.UTF8_ENCODING

			result := make([]byte, len(testValue))

			n, _, err := decoder.Read(result)
			checkResult(f, testValue, r.valid, result, err, n)
		}
	}
}

func TestUtf8RangesSingleValueFromReader(f *testing.T) {
	for _, r := range utf8Ranges {
		combinations := generateAllBoundCombinations(r.bounds)

		for _, testValue := range combinations {
			decoder := reader.NewUtfDecoderForReader(bytes.NewReader(testValue))
			decoder.Encoding = reader.UTF8_ENCODING

			result := make([]byte, 4)

			n, _, err := decoder.Read(result)
			checkResult(f, testValue, r.valid, result, err, n)
		}
	}
}
