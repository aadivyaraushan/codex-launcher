package recipient

import "errors"

var (
	ErrNone     = errors.New("recipient: no match")
	ErrAmbiguous = errors.New("recipient: ambiguous name")
)

func ExactOne(normalized string, candidates []string) (string, error) {
	var hits []string
	for _, c := range candidates {
		if c == normalized {
			hits = append(hits, c)
		}
	}
	switch len(hits) {
	case 0:
		return "", ErrNone
	case 1:
		return hits[0], nil
	default:
		return "", ErrAmbiguous
	}
}
