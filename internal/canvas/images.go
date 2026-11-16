package canvas

// CellRect describes a visible rectangle in terminal cells.
type CellRect struct {
	X int
	Y int
	W int
	H int
}

// PixelImage is browser graphics metadata, separate from terminal escape text.
type PixelImage struct {
	CellRect
	Asset   string
	Color   string
	Opacity float64
	Clips   []CellRect
}

// PlaceImage records a cached image over these cells. Later writes occlude it,
// including spaces used to clear an overlay, without any terminal protocol.
func (b *Buffer) PlaceImage(image PixelImage) {
	if b.pixelOwners == nil {
		b.pixelOwners = make([]int, b.w*b.h)
	}
	b.pixelImages = append(b.pixelImages, image)
	owner := len(b.pixelImages)
	for y := max(0, image.Y); y < min(b.h, image.Y+image.H); y++ {
		for x := max(0, image.X); x < min(b.w, image.X+image.W); x++ {
			b.pixelOwners[y*b.w+x] = owner
		}
	}
}

// PixelImages returns only the regions still visible in the completed frame.
func (b *Buffer) PixelImages() []PixelImage {
	out := make([]PixelImage, 0, len(b.pixelImages))
	for index, image := range b.pixelImages {
		image.Clips = nil
		for y := max(0, image.Y); y < min(b.h, image.Y+image.H); y++ {
			for x := max(0, image.X); x < min(b.w, image.X+image.W); {
				if b.pixelOwners[y*b.w+x] != index+1 {
					x++
					continue
				}
				start := x
				for x < min(b.w, image.X+image.W) && b.pixelOwners[y*b.w+x] == index+1 {
					x++
				}
				merged := false
				for i := range image.Clips {
					clip := &image.Clips[i]
					if clip.X == start && clip.W == x-start && clip.Y+clip.H == y {
						clip.H++
						merged = true
						break
					}
				}
				if !merged {
					image.Clips = append(image.Clips, CellRect{start, y, x - start, 1})
				}
			}
		}
		if len(image.Clips) > 0 {
			out = append(out, image)
		}
	}
	return out
}
