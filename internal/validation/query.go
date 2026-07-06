package validation

import "fmt"

// searchQueryMaxLength bounds velo_search_clients' free-text query
// field. Velociraptor's own client-search syntax (hostname/IP/label
// globs, "label:x", etc.) has no legitimate need for very long input,
// and an unbounded string is needless attack surface even though it is
// always passed as a bound protobuf field, never concatenated into VQL.
const searchQueryMaxLength = 256

// SearchQuery validates the free-text query passed to
// velo_search_clients. Empty is valid (it means "no filter"). This
// rejects NUL bytes and other C0 control characters and caps length; it
// does not validate Velociraptor's client-search grammar itself, since
// that grammar is not this project's to define.
func SearchQuery(q string) error {
	if len(q) > searchQueryMaxLength {
		return fmt.Errorf("validation: search query exceeds %d characters", searchQueryMaxLength)
	}
	for _, r := range q {
		if r == 0 || (r < 0x20 && r != '\t') {
			return fmt.Errorf("validation: search query contains a control character")
		}
	}
	return nil
}
