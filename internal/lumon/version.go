package lumon

// Version is what this terminal says it is. The about box reads it.
//
// cmd/lumon keeps its own literal because -X can only stamp a variable whose
// initialiser is a constant string, and copies it here at startup.
// TestTheVersionIsTheSameInBothPlaces fails if the two literals differ.
var Version = "3.0.0"
