package git

import "fmt"

// Range holds two resolved commit SHAs for comparison.
type Range struct {
	A string
	B string
}

// NewRange resolves two refs into their commit SHAs.
func NewRange(a, b string) (*Range, error) {
	shas, err := RevParse(a, b)
	if err != nil {
		return nil, err
	}
	if len(shas) != 2 {
		return nil, fmt.Errorf("failed to resolve refs: %s, %s", a, b)
	}
	return &Range{A: shas[0], B: shas[1]}, nil
}

// IsIdentical returns true when both refs point to the same commit.
func (r *Range) IsIdentical() bool {
	return r.A == r.B
}

// IsAncestor returns true when A is an ancestor of B,
// meaning B is strictly ahead and a fast-forward is possible.
func (r *Range) IsAncestor() bool {
	return Run("merge-base", "--is-ancestor", r.A, r.B)
}
