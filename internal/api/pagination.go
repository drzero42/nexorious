package api

import (
	"errors"
	"strconv"
)

// pageNumber defaults invalid pages to one. Positive values beyond the
// integer range saturate, so they still select a page past the end.
func pageNumber(param string) int {
	page, err := strconv.Atoi(param)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		return 1
	}
	return max(page, 1)
}

// pageOffset multiplies only when the requested page is within the result
// set, so a huge page number cannot wrap around to an earlier page.
func pageOffset(page, pageSize int, total int64) (int64, bool) {
	index := int64(page - 1)
	size := int64(pageSize)
	if total <= 0 || index > (total-1)/size {
		return 0, false
	}
	return index * size, true
}
