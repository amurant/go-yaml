// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reader

import (
	"errors"
	"io"
)

type Encoding uint8

const (
	AUTO_DETECT_ENCODING Encoding = iota
	UTF8_ENCODING
	UTF16BE_ENCODING
	UTF16LE_ENCODING
	UTF32BE_ENCODING
	UTF32LE_ENCODING
)

type UtfDecoder struct {
	input_reader io.Reader // File input data.
	eof_reached  bool      // True if EOF reached.
	raw_buffer   []byte    // Raw buffer, holds buffer allocation (if any).
	input_buffer []byte    // Input buffer, points to current input position.
	spill_buffer [4]byte   // Buffer used to store non-bom bytes, or unfinished char.

	Encoding Encoding
}

func NewUtfDecoderForBuffer(buffer []byte) UtfDecoder {
	return UtfDecoder{
		input_reader: nil,
		input_buffer: buffer,
		eof_reached:  true,
		Encoding:     AUTO_DETECT_ENCODING,
	}
}

const input_buffer_size = 512

func NewUtfDecoderForReader(reader io.Reader) UtfDecoder {
	return UtfDecoder{
		input_reader: reader,
		input_buffer: nil,
		Encoding:     AUTO_DETECT_ENCODING,
	}
}

var _ = &UtfDecoder{}

// WARNING: in case len(out_buffer) < length of the char,
// bytes_read will be 0; please only use with len(out_buffer) >= 4
func (u *UtfDecoder) Read(out_buffer []byte) (bytes_read int, runes_read int, err error) {
	if len(out_buffer) == 0 {
		return 0, 0, nil
	}

	if u.eof_reached && len(u.input_buffer) == 0 {
		return 0, 0, io.EOF
	}

	if u.Encoding == AUTO_DETECT_ENCODING {
		// If we have a reader, we need to read the first
		// few bytes to see if we have a bom. Otherwise,
		// we can just use the input_buffer we were given.
		if u.input_reader != nil {
			if n, err := u.input_reader.Read(u.spill_buffer[:4]); err != nil && err != io.EOF {
				return 0, 0, err
			} else {
				if err != nil || n < 4 {
					u.eof_reached = true
				}
				u.input_buffer = u.spill_buffer[:n]
			}
		}

		// Detect UTF BOM and skip it.
		n_input_buffer := len(u.input_buffer)
		if n_input_buffer >= 4 && ((u.input_buffer[0] == 0x00 && u.input_buffer[1] == 0x00 && u.input_buffer[2] == 0xfe && u.input_buffer[3] == 0xff) ||
			(u.input_buffer[0] == 0x00 && u.input_buffer[1] == 0x00 && u.input_buffer[2] == 0x00)) {
			u.Encoding = UTF32BE_ENCODING
			u.input_buffer = u.input_buffer[4:]
		} else if n_input_buffer >= 4 && ((u.input_buffer[0] == 0xff && u.input_buffer[1] == 0xfe && u.input_buffer[2] == 0x00 && u.input_buffer[3] == 0x00) ||
			(u.input_buffer[1] == 0x00 && u.input_buffer[2] == 0x00 && u.input_buffer[3] == 0x00)) {
			u.Encoding = UTF32LE_ENCODING
			u.input_buffer = u.input_buffer[4:]
		} else if n_input_buffer >= 3 && u.input_buffer[0] == 0xef && u.input_buffer[1] == 0xbb && u.input_buffer[2] == 0xbf {
			u.Encoding = UTF8_ENCODING
			u.input_buffer = u.input_buffer[3:]
		} else if n_input_buffer >= 2 && ((u.input_buffer[0] == 0xfe && u.input_buffer[1] == 0xff) ||
			(u.input_buffer[0] == 0x00)) {
			u.Encoding = UTF16BE_ENCODING
			u.input_buffer = u.input_buffer[2:]
		} else if n_input_buffer >= 2 && ((u.input_buffer[0] == 0xff && u.input_buffer[1] == 0xfe) ||
			(u.input_buffer[1] == 0x00)) {
			u.Encoding = UTF16LE_ENCODING
			u.input_buffer = u.input_buffer[2:]
		} else {
			u.Encoding = UTF8_ENCODING
		}
	}

	switch u.Encoding {
	case UTF8_ENCODING:
		if len(u.input_buffer) > 0 {
			bytes_read = copy(out_buffer, u.input_buffer)
			u.input_buffer = u.input_buffer[bytes_read:]
		}

		if bytes_read < len(out_buffer) && u.input_reader != nil {
			n, readErr := u.input_reader.Read(out_buffer[bytes_read:])
			if readErr != nil || n < len(out_buffer[bytes_read:]) {
				u.eof_reached = true
			}
			bytes_read += n
			err = readErr
		}

		{
			n, nRunes, checkErr := u.CheckUtf8(out_buffer[:bytes_read], u.eof_reached)
			// If an unfinished character was read, copy the unfinished part to
			// the spill buffer and use it in a next call.
			if checkErr != nil && n < bytes_read {
				spill := copy(u.spill_buffer[:], u.input_buffer)
				spill += copy(u.spill_buffer[spill:], out_buffer[n:bytes_read])
				u.input_buffer = u.spill_buffer[:spill]
			}

			bytes_read = n
			runes_read = nRunes
			if checkErr != nil {
				err = checkErr
			}
		}
	case UTF16BE_ENCODING, UTF16LE_ENCODING:
		// If the input reader has not yet got a big buffer to write into,
		// create such a buffer
		if u.input_reader != nil && cap(u.input_buffer) < input_buffer_size {
			u.raw_buffer = make([]byte, input_buffer_size) // Allocate raw_buffer
			nCopied := copy(u.raw_buffer, u.input_buffer)  // Copy data in input_buffer to raw_buffer (max 4 bytes of non-BOM data)
			u.input_buffer = u.raw_buffer[:nCopied]        // Set input_buffer to point to raw_buffer
		}

		// Loop until full out_buffer is filled or
		// we reach EOF/ the end of the input_buffer
		for bytes_read < len(out_buffer) && (len(u.input_buffer) > 0 || !u.eof_reached) {
			// Worst case a 2 byte UTF16 character can be represented
			// by a 1 byte UTF8 character, so we want to read perferably
			// if we have less than twice as many bytes as the output buffer size
			if u.input_reader != nil && len(u.input_buffer) < len(out_buffer)*2 {
				free_buffer := u.input_buffer[len(u.input_buffer):cap(u.input_buffer)] // Select the free part of the buffer
				n, readErr := u.input_reader.Read(free_buffer)                         // Fill up the free part of the buffer
				if readErr != nil || n < len(free_buffer) {
					u.eof_reached = true
				}
				u.input_buffer = u.input_buffer[:len(u.input_buffer)+n] // Set input_buffer to point to the full buffer
				err = readErr
			}

			if len(u.input_buffer) > 0 {
				nDst, nSrc, transErr := u.TransformUtf16(out_buffer[bytes_read:], u.input_buffer, u.eof_reached)
				bytes_read += nDst
				u.input_buffer = u.input_buffer[nSrc:]
				if transErr != nil {
					err = transErr
				}
			}
		}

		var checkErr error
		bytes_read, runes_read, checkErr = u.CheckUtf8(out_buffer[:bytes_read], u.eof_reached)
		if checkErr != nil {
			err = checkErr
		}
	case UTF32BE_ENCODING, UTF32LE_ENCODING:
		// If the input reader has not yet got a big buffer to write into,
		// create such a buffer
		if u.input_reader != nil && cap(u.input_buffer) < input_buffer_size {
			u.raw_buffer = make([]byte, input_buffer_size) // Allocate raw_buffer
			nCopied := copy(u.raw_buffer, u.input_buffer)  // Copy data in input_buffer to raw_buffer (max 4 bytes of non-BOM data)
			u.input_buffer = u.raw_buffer[:nCopied]        // Set input_buffer to point to raw_buffer
		}

		// Loop until full out_buffer is filled or
		// we reach EOF/ the end of the input_buffer
		for bytes_read < len(out_buffer) && (len(u.input_buffer) > 0 || !u.eof_reached) {
			// Worst case a 4 byte UTF32 character can be represented
			// by a 1 byte UTF8 character, so we want to read 4 times as
			// many bytes as the output buffer size
			if u.input_reader != nil && len(u.input_buffer) < len(out_buffer)*4 {
				free_buffer := u.input_buffer[len(u.input_buffer):cap(u.input_buffer)] // Select the free part of the buffer
				n, readErr := u.input_reader.Read(free_buffer)                         // Fill up the free part of the buffer
				if readErr == io.EOF || n < len(free_buffer) {
					u.eof_reached = true
				}
				u.input_buffer = u.input_buffer[:len(u.input_buffer)+n] // Set input_buffer to point to the full buffer
				err = readErr
			}

			if len(u.input_buffer) > 0 {
				nDst, nSrc, transErr := u.TransformUtf32(out_buffer[bytes_read:], u.input_buffer, u.eof_reached)
				bytes_read += nDst
				u.input_buffer = u.input_buffer[nSrc:]
				if transErr != nil {
					err = transErr
				}
			}
		}

		var checkErr error
		bytes_read, runes_read, checkErr = u.CheckUtf8(out_buffer[:bytes_read], u.eof_reached)
		if checkErr != nil {
			err = checkErr
		}
	default:
		panic("unreachable")
	}

	if u.eof_reached && len(u.input_buffer) == 0 {
		if err == nil && bytes_read == 0 {
			err = io.EOF
		}
		u.input_reader = nil
		u.raw_buffer = nil
		u.input_buffer = nil
	}

	return bytes_read, runes_read, err
}

const (
	runeError = '\uFFFD'     // Unicode replacement character
	maxRune   = '\U0010FFFF' // Maximum valid Unicode code point.
	runeSelf  = 0x80         // characters below RuneSelf are represented as themselves in a single byte.
)

var ErrInvalidUtf8 = errors.New("invalid utf8 character encountered")

// Decode a UTF-8 character.  Check RFC 3629
// (http://www.ietf.org/rfc/rfc3629.txt) for more details.
//
// The following table (taken from the RFC) is used for
// decoding.
//
//    Char. number range |        UTF-8 octet sequence
//      (hexadecimal)    |              (binary)
//   --------------------+------------------------------------
//   0000 0000-0000 007F | 0xxxxxxx
//   0000 0080-0000 07FF | 110xxxxx 10xxxxxx
//   0000 0800-0000 FFFF | 1110xxxx 10xxxxxx 10xxxxxx
//   0001 0000-0010 FFFF | 11110xxx 10xxxxxx 10xxxxxx 10xxxxxx
//
// Additionally, the characters in the range 0xD800-0xDFFF
// are prohibited as they are reserved for use with UTF-16
// surrogate pairs.
func (UtfDecoder) CheckUtf8(buffer []byte, atEOF bool) (nSrc int, nSrcRunes int, err error) {
	n := len(buffer)
	for nSrc < n {
		c := buffer[nSrc]

		// If character is below runeSelf, it is represented as itself in a single byte,
		// all following checks can be skipped
		if c < runeSelf {
			nSrc++
			nSrcRunes++
			continue
		}

		var width int

		// Determine the length of the UTF-8 sequence.
		switch {
		case c&0b10000000 == 0b00000000: // 0xxxxxxx
			width = 1
		case c&0b11100000 == 0b11000000: // 110xxxxx
			width = 2
		case c&0b11110000 == 0b11100000: // 1110xxxx
			width = 3
		case c&0b11111000 == 0b11110000: // 11110xxx
			width = 4
		default:
			goto handleInvalid // invalid starter byte
		}

		if nSrc+width > n {
			if !atEOF {
				break
			}
			goto handleInvalid
		}

		// Check the length of the sequence against the value.
		switch {
		case width == 1:
		case width == 2 &&
			(buffer[nSrc+1]&0b11000000 == 0b10000000) && // 10xxxxxx
			(buffer[nSrc+0]&0b00011110 != 0): // value >= 0x80
		case width == 3 &&
			(buffer[nSrc+1]&0b11000000 == 0b10000000) && // 10xxxxxx
			(buffer[nSrc+2]&0b11000000 == 0b10000000) && // 10xxxxxx
			(buffer[nSrc+0]&0b00001111 != 0b00000000 || buffer[nSrc+1]&0b00100000 != 0b00000000) && // value >= 0x800
			(buffer[nSrc+0]&0b00001111 != 0b00001101 || buffer[nSrc+1]&0b00100000 == 0b00000000): // !(value >= 0xD800 && value <= 0xDFFF)
		case width == 4 &&
			(buffer[nSrc+1]&0b11000000 == 0b10000000) && // 10xxxxxx
			(buffer[nSrc+2]&0b11000000 == 0b10000000) && // 10xxxxxx
			(buffer[nSrc+3]&0b11000000 == 0b10000000) && // 10xxxxxx
			(buffer[nSrc+0]&0b00000111 != 0b00000000 || buffer[nSrc+1]&0b00110000 != 0b00000000) && // value >= 0x10000
			(buffer[nSrc+0]&0b00000100 == 0b00000000 || (buffer[nSrc+0]&0b00000011 == 0b00000000 && buffer[nSrc+1]&0b00110000 == 0b00000000)): // value < 0x10FFFF
		default:
			goto handleInvalid // invalid starter byte
		}

		nSrc += width
		nSrcRunes++
		continue

	handleInvalid:
		return nSrc, nSrcRunes, ErrInvalidUtf8
	}

	return nSrc, nSrcRunes, nil
}

const (
	surrogateMin = 0xD800
	surrogateMax = 0xDFFF
)

// RuneLen returns the number of bytes required to encode the rune.
// It returns -1 if the rune is not a valid value to encode in UTF-8.
func runeLen(r rune) int {
	// Negative values are erroneous. Making it unsigned addresses the problem.
	switch i := uint32(r); {
	case i > maxRune, surrogateMin <= i && i <= surrogateMax:
		return -1
	case i <= 0x7F:
		return 1
	case i <= 0x7FF:
		return 2
	case i <= 0xFFFF:
		return 3
	default:
		return 4
	}
}

// EncodeRune writes into p (which must be large enough) the UTF-8 encoding of the rune.
// If the rune is out of range, it writes the encoding of RuneError.
// It returns the number of bytes written.
func encodeRune(p []byte, r rune) int {
	// Negative values are erroneous. Making it unsigned addresses the problem.
	switch i := uint32(r); {
	case i <= 0x7F:
		// 0000 0000-0000 007F . 0xxxxxxx
		p[0] = byte(r>>0) & 0b01111111
		return 1
	case i <= 0x7FF:
		// 0000 0080-0000 07FF . 110xxxxx 10xxxxxx
		_ = p[1] // eliminate bounds checks
		p[0] = 0b11000000 | byte(r>>6)
		p[1] = 0b10000000 | byte(r>>0)&0b00111111
		return 2
	case i > maxRune, surrogateMin <= i && i <= surrogateMax:
		r = runeError
		fallthrough
	case i <= 0xFFFF:
		// 0000 0800-0000 FFFF . 1110xxxx 10xxxxxx 10xxxxxx
		_ = p[2] // eliminate bounds checks
		p[0] = 0b11100000 | byte(r>>12)
		p[1] = 0b10000000 | byte(r>>06)&0b00111111
		p[2] = 0b10000000 | byte(r>>00)&0b00111111
		return 3
	default:
		// 0001 0000-0010 FFFF . 11110xxx 10xxxxxx 10xxxxxx 10xxxxxx
		_ = p[3] // eliminate bounds checks
		p[0] = 0b11110000 | byte(r>>18)
		p[1] = 0b10000000 | byte(r>>12)&0b00111111
		p[2] = 0b10000000 | byte(r>>06)&0b00111111
		p[3] = 0b10000000 | byte(r>>00)&0b00111111
		return 4
	}
}

var ErrInvalidUtf16 = errors.New("invalid utf16 character encountered")

// Decode a UTF-16 character.  Check RFC 2781
// (http://www.ietf.org/rfc/rfc2781.txt).
//
// Normally, two subsequent bytes describe a Unicode
// character.  However a special technique (called a
// surrogate pair) is used for specifying character
// values larger than 0xFFFF.
//
// A surrogate pair consists of two pseudo-characters:
//      high surrogate area (0xD800-0xDBFF)
//      low surrogate area (0xDC00-0xDFFF)
//
// The following formulas are used for decoding
// and encoding characters using surrogate pairs:
//
//  U  = U' + 0x10000   (0x01 00 00 <= U <= 0x10 FF FF)
//  U' = yyyyyyyyyyxxxxxxxxxx   (0 <= U' <= 0x0F FF FF)
//  W1 = 110110yyyyyyyyyy
//  W2 = 110111xxxxxxxxxx
//
// where U is the character value, W1 is the high surrogate
// area, W2 is the low surrogate area.
func (u *UtfDecoder) TransformUtf16(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	if u.Encoding != UTF16BE_ENCODING && u.Encoding != UTF16LE_ENCODING {
		return 0, 0, nil
	}

	var r rune
	var dSize, sSize int

	n := len(src)
	for nSrc < n {
		if nSrc+1 >= n {
			if !atEOF {
				return nDst, nSrc, nil
			}
			return nDst, nSrc, ErrInvalidUtf16
		}

		x := uint16(src[nSrc+0])<<8 | uint16(src[nSrc+1])
		if u.Encoding == UTF16LE_ENCODING {
			x = x>>8 | x<<8
		}
		r = rune(x)
		sSize = 2

		if r&0b11111000_00000000 == 0b11011000_00000000 {
			if nSrc+3 >= n {
				if !atEOF {
					return nDst, nSrc, nil
				}
				return nDst, nSrc, ErrInvalidUtf16
			}

			x := uint16(src[nSrc+2])<<8 | uint16(src[nSrc+3])
			if u.Encoding == UTF16LE_ENCODING {
				x = x>>8 | x<<8
			}
			r2 := rune(x)

			// Save for next iteration if it is not a high surrogate.
			if r2&0b11111100_00000000 == 0b11011100_00000000 {
				r = ((r & 0b00000011_11111111) << 10) | (r2 & 0b00000011_11111111) + 0x10000
				sSize = 4
			}
		}

		if dSize = runeLen(r); dSize < 0 {
			r, dSize = runeError, 3
		}

		if nDst+dSize > len(dst) {
			break
		}

		nDst += encodeRune(dst[nDst:], r)
		nSrc += sSize
	}

	return nDst, nSrc, nil
}

var ErrInvalidUtf32 = errors.New("invalid utf32 character encountered")

// Decode a UTF-32 character.
func (u *UtfDecoder) TransformUtf32(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	if u.Encoding != UTF32BE_ENCODING && u.Encoding != UTF32LE_ENCODING {
		return 0, 0, nil
	}

	var r rune
	var dSize, sSize int

	n := len(src)
	for nSrc < n {
		if nSrc+3 >= n {
			if !atEOF {
				return nDst, nSrc, nil
			}
			return nDst, nSrc, ErrInvalidUtf32
		}

		x := uint32(src[nSrc+0])<<24 | uint32(src[nSrc+1])<<16 | uint32(src[nSrc+2])<<8 | uint32(src[nSrc+3])
		if u.Encoding == UTF32LE_ENCODING {
			x = x>>24 | (x >> 8 & 0x0000FF00) | (x << 8 & 0x00FF0000) | x<<24
		}
		r, sSize = rune(x), 4

		if dSize = runeLen(r); dSize < 0 {
			r, dSize = runeError, 3
		}

		if nDst+dSize > len(dst) {
			break
		}

		nDst += encodeRune(dst[nDst:], r)
		nSrc += sSize
	}

	return nDst, nSrc, err
}
