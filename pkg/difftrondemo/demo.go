// Package difftrondemo is a throwaway sample used to demonstrate difftron's
// delta-coverage gate on a pull request. Safe to delete.
package difftrondemo

// Covered is exercised by the test in this package, so its changed lines
// should report as covered.
func Covered(a, b int) int {
	return a + b
}

// Uncovered has no test, so difftron should flag these changed lines as
// uncovered and pull the PR below the threshold.
func Uncovered(a, b int) int {
	if a > b {
		return a - b
	}
	return b - a
}
