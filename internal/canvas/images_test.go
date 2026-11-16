package canvas

import "testing"

func TestPixelImageClipsLaterWritesIncludingBlankOverlays(t *testing.T) {
	b := NewBuffer(10, 6)
	b.PlaceImage(PixelImage{CellRect: CellRect{1, 1, 4, 3}, Asset: "mark"})
	images := b.PixelImages()
	if len(images) != 1 || len(images[0].Clips) != 1 || images[0].Clips[0] != (CellRect{1, 1, 4, 3}) {
		t.Fatalf("whole image: %+v", images)
	}
	b.Set(2, 2, ' ', Attr{})
	images = b.PixelImages()
	area := 0
	for _, clip := range images[0].Clips {
		area += clip.W * clip.H
		if clip.X <= 2 && clip.X+clip.W > 2 && clip.Y <= 2 && clip.Y+clip.H > 2 {
			t.Fatal("blank overlay did not occlude image")
		}
	}
	if area != 11 {
		t.Fatalf("visible area=%d, want11", area)
	}
	b.Clear()
	if len(b.PixelImages()) != 0 {
		t.Fatal("image survived frame reset")
	}
}

func TestPixelImagesRespectViewportAndLaterImages(t *testing.T) {
	b := NewBuffer(4, 4)
	b.PlaceImage(PixelImage{CellRect: CellRect{-1, -1, 3, 3}, Asset: "first"})
	b.PlaceImage(PixelImage{CellRect: CellRect{0, 0, 2, 2}, Asset: "second"})
	images := b.PixelImages()
	if len(images) != 1 || images[0].Asset != "second" || images[0].Clips[0] != (CellRect{0, 0, 2, 2}) {
		t.Fatalf("overlap: %+v", images)
	}
}
