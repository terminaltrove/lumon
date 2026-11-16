//go:build !race

package lumon

// raceEnabled reports whether the binary was built with the race detector.
// Its counterpart is race_on_test.go.
const raceEnabled = false
