package smug

import (
	"io/ioutil"
	"net/http"
	"strconv"
)

func ChunkSplit(body string, limit int) []string {
	result := []string{}
	var charSlice []rune
	// push characters to slice
	for _, char := range body {
		charSlice = append(charSlice, char)
	}

	for len(charSlice) >= 1 {
		// convert slice/array back to string
		result = append(result, string(charSlice[:limit]))
		// discard the elements that were copied over to result
		charSlice = charSlice[limit:]
		// change the limit to cater for the last few words in charSlice
		if len(charSlice) < limit {
			limit = len(charSlice)
		}
	}
	return result
}

func FetchUrl(url string) ([]byte, error) {
	// Get the data
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	text, err := ioutil.ReadAll(resp.Body)
	return text, err
}

func fmtInt64(i int64) string {
	return strconv.FormatInt(i, 10)
}
