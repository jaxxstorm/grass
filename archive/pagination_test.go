package archive

import (
	"math"
	"net/url"
	"strconv"
	"testing"
)

func TestPaginationOffsetOverflow(t *testing.T) {
	maxPage := math.MaxInt/25 + 1
	for _, page := range []int{maxPage + 1, math.MaxInt} {
		if _, err := ParseQuery(url.Values{"page": {strconv.Itoa(page)}, "page_size": {"25"}}); err == nil {
			t.Fatalf("accepted overflowing page %d", page)
		}
	}
	if _, err := ParseQuery(url.Values{"page": {strconv.Itoa(maxPage)}, "page_size": {"25"}}); err != nil {
		t.Fatal(err)
	}
}
