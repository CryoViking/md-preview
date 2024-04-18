package main

import "slices"

func Shrink_double_linefeeds(data []byte) []byte {
	result := make([]byte, 0, len(data))

	check_char := func(c byte) bool {
		return c == '\n' || c == '\r'
	}

	for i, b := range data {
		// If we have 2 in a row, skip the second one
		if check_char(b) && check_char(data[i-1]) {
			continue
		}
		result = append(result, b)
	}
	return slices.Clip(result)
}
