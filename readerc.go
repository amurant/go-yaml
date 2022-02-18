//
// Copyright (c) 2011-2019 Canonical Ltd
// Copyright (c) 2006-2010 Kirill Simonov
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies
// of the Software, and to permit persons to whom the Software is furnished to do
// so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package yaml

import "io"

// Set the reader error and return 0.
func yaml_parser_set_reader_error(parser *yaml_parser_t, problem string, offset int, value int) bool {
	parser.error = yaml_READER_ERROR
	parser.problem = problem
	parser.problem_offset = offset
	parser.problem_value = value
	return false
}

// Update the raw buffer.
func yaml_parser_update_buffer(parser *yaml_parser_t, length int) bool {
	if len(parser.buffer)-parser.buffer_pos >= length {
		return true
	}

	// Move the remaining bytes in the buffer to the beginning.
	if parser.buffer_pos > 0 {
		copy(parser.buffer, parser.buffer[parser.buffer_pos:])
	}
	parser.buffer = parser.buffer[:len(parser.buffer)-parser.buffer_pos]
	parser.buffer_pos = 0

	for len(parser.buffer)-parser.buffer_pos < length {
		free_buffer := parser.buffer[len(parser.buffer):cap(parser.buffer)]
		n_bytes, n_runes, err := parser.reader.Read(free_buffer)
		eof := err == io.EOF
		parser.buffer = parser.buffer[:len(parser.buffer)+n_bytes]
		parser.unread += n_runes

		if err != nil && err != io.EOF {
			return yaml_parser_set_reader_error(parser, "input error: "+err.Error(), parser.offset, -1)
		}

		if eof {
			// In case we are at the end of the stream,
			// append the fake "\0" characters.
			n_requested := parser.buffer_pos + length
			n_content := len(parser.buffer)
			if n_requested > n_content {
				parser.buffer = parser.buffer[:n_requested]

				for i := n_content; i < n_requested; i++ {
					parser.buffer[i] = 0
				}
			}

			break
		}
	}

	return true
}
