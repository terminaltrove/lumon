//go:build race

package lumon

// raceEnabled reports whether the binary was built with the race detector. The
// throughput test skips under -race, where the instrumentation costs nearly an
// order of magnitude. Only a build tag can tell.
const raceEnabled = true
