package lumon

// The address readout along the bottom of the screen. The show puts a pair of
// hex addresses under the bins (0xFFE1E7A0 : 0xFFF0F3A7). Here the left is the
// cell the cursor is on and the right is one past the last cell of the visible
// block.
//
// It is a pure function of position and file, and reads no clock: it changes
// only when the refiner moves, and two refiners on the same cell of the same
// file read the same pair whatever their terminal size.

const (
	// addressBase is where a macrodata file is mapped, high in a 32-bit space
	// like the show's addresses, and low enough that a 100x100 grid at
	// addressStride still fits without wrapping.
	//
	// It must stay typed: 0xFFC00000 overflows an int on 32-bit builds, and an
	// untyped constant would take int from the expression it is used in.
	addressBase uint32 = 0xFFC00000

	// addressStride is how many bytes a cell is taken to occupy. It is large
	// so that a step to the next cell visibly changes the third hex digit, and
	// a power of two so cell boundaries are round numbers in hex.
	addressStride = 0x100

	// addressEpochStride separates one file from the next, so a new file shows
	// a different range.
	addressEpochStride = 0x00010000
)

// AddressRange is the pair of addresses shown: the cursor's cell, and one past
// the end of the visible block. The first follows the cursor, the second the
// viewport.
func AddressRange(l *Layout, cursorX, cursorY, epoch int) (at, end uint32) {
	base := addressBase + uint32(epoch)*addressEpochStride

	cell := cursorY*GridSize + cursorX
	last := (l.ViewportY+l.ViewportH-1)*GridSize + l.ViewportX + l.ViewportW

	return base + uint32(cell*addressStride), base + uint32(last*addressStride)
}

// formatAddress writes 0x and eight zero-padded uppercase hex digits, so the
// readout never changes width.
func formatAddress(v uint32) string { return string(appendAddress(nil, v)) }

// appendAddress writes one address onto a byte slice, without allocating.
func appendAddress(b []byte, v uint32) []byte {
	const hex = "0123456789ABCDEF"
	b = append(b, '0', 'x')
	for shift := 28; shift >= 0; shift -= 4 {
		b = append(b, hex[(v>>uint(shift))&0xF])
	}
	return b
}

// addressLine is the full readout. It is cached on its range because it is
// drawn every frame and changes only when the refiner moves.
func (m *Model) addressLine() string {
	epoch := 0
	if m.Room != nil {
		epoch = m.Room.Epoch()
	}
	start, end := AddressRange(m.Layout, m.CursorX, m.CursorY, epoch)

	if m.addrLine != "" && m.addrFrom == start && m.addrTo == end {
		return m.addrLine
	}

	b := make([]byte, 0, 23)
	b = appendAddress(b, start)
	b = append(b, ' ', ':', ' ')
	b = appendAddress(b, end)

	m.addrFrom, m.addrTo, m.addrLine = start, end, string(b)
	return m.addrLine
}
