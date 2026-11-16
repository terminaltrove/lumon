package agent

// alive is false on Windows, where the game is never opened for an agent.
func alive(int) bool { return false }

// Detach is unsupported on Windows.
func Detach(string, ...string) error { return ErrNoTerminal }

// DetachWith is unsupported on Windows.
func DetachWith([]byte, string, ...string) error { return ErrNoTerminal }
