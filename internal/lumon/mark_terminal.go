package lumon

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"strconv"
	"strings"
	"sync"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
)

// The mark as a picture for terminals that draw images, kitty and the iTerm2
// inline protocol. The browser draws it as a texture from its own copy of the
// asset and needs none of this; see mark_js.go.

// markMask is the mark as coverage: opaque where the logo is, transparent
// elsewhere, with no colour of its own. The session tints it in its own
// Theme.Primary, so `t` restyles the mark with everything else.
//
// Made with:
//
//	magick lumon_223.png -colorspace gray -alpha off -resize 400x206 \
//	    -clamp -auto-level mask.png
//	magick -size 400x206 xc:white mask.png -alpha off \
//	    -compose CopyOpacity -composite -strip \
//	    -define png:compression-level=9 assets/lumon.mark.png
//
// 400 wide oversamples the twenty-cell box even on a dense display; kitty
// scales it to fit. -auto-level is needed because resampling the thin source
// stroke leaves the most opaque pixel at 73%, which draws as a faint smear.
// -clamp must come first: the resize filter undershoots below zero at hard
// edges, and auto-levelling from there puts an 11% wash over the entire
// rectangle. The mask must be transparent where the mark is not, since it is
// drawn over the file bar and the about box's frame.
//
//go:embed assets/lumon.mark.png
var markMask []byte

var (
	markOnce  sync.Once
	markAlpha *image.Alpha
)

// markShape decodes the mask once and keeps only its alpha channel, which is
// all the tint reads.
func markShape() *image.Alpha {
	markOnce.Do(func() {
		src, err := png.Decode(bytes.NewReader(markMask))
		if err != nil {
			return // not fatal: the braille mark is the fallback for this too
		}
		b := src.Bounds()
		a := image.NewAlpha(b)
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				_, _, _, av := src.At(x, y).RGBA()
				a.SetAlpha(x, y, color.Alpha{A: uint8(av >> 8)})
			}
		}
		markAlpha = a
	})
	return markAlpha
}

// markImage is the mark tinted in this session's own colour. It is rebuilt
// only when the theme's name changes, which is what `t` changes.
func (m *Model) markImage() image.Image {
	if m.markFor == m.Theme.Name && m.markImg != nil {
		return m.markImg
	}
	shape := markShape()
	if shape == nil {
		return nil
	}

	// An unset role is the terminal's own foreground, which cannot be named
	// here; use white, as an unset default means a dark theme.
	r, g, b, ok := m.Theme.Primary.Parts()
	if !ok {
		r, g, b = 0xFF, 0xFF, 0xFF
	}
	// Paletted, at markLevels steps of the one colour. The inline protocol
	// resends the picture every time it is drawn: full colour is 26KB, a
	// 32-step palette 8.9KB, with no visible difference on an antialiased
	// hairline. kitty uses the same encoding.
	pal := make(color.Palette, markLevels)
	for i := range pal {
		pal[i] = color.NRGBA{R: r, G: g, B: b, A: uint8(i * 0xFF / (markLevels - 1))}
	}
	bounds := shape.Bounds()
	out := image.NewPaletted(bounds, pal)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			a := int(shape.AlphaAt(x, y).A)
			out.SetColorIndex(x, y, uint8(a*(markLevels-1)/0xFF))
		}
	}

	m.markFor, m.markImg, m.markSent = m.Theme.Name, out, false
	m.markInline = ""
	return m.markImg
}

// markInlineSeq is the picture as an iTerm2 inline image, built once per theme.
// The protocol has no equivalent of kitty's C=1, so drawing advances the
// cursor; DECSC and DECRC put it back. The image is cell content, so the
// buffer Reserves its cells instead of painting them (a space would rub it out).
func (m *Model) markInlineSeq() string {
	if m.markInline != "" && m.markFor == m.Theme.Name {
		return m.markInline
	}
	img := m.markImage()
	if img == nil {
		return ""
	}
	var raw bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&raw, img); err != nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\x1b7") // DECSC
	sb.WriteString("\x1b]1337;File=inline=1;size=")
	sb.WriteString(strconv.Itoa(raw.Len()))
	sb.WriteString(";width=")
	sb.WriteString(strconv.Itoa(MarkW))
	sb.WriteString(";height=")
	sb.WriteString(strconv.Itoa(MarkH))
	// Fill the box: cells are not square, so preserving the aspect would
	// letterbox the mark inside its own footprint.
	sb.WriteString(";preserveAspectRatio=0:")
	sb.WriteString(base64.StdEncoding.EncodeToString(raw.Bytes()))
	sb.WriteString("\a")
	sb.WriteString("\x1b8") // DECRC

	m.markInline = sb.String()
	return m.markInline
}

// markTransmit is the sequence that puts the picture on the terminal, or empty
// once it is already there. It is sent once a session: kitty's
// transmit-and-display would resend the payload each time Bubble Tea repaints
// the file bar's rows, and the payload is about twice a typical frame
// (BenchmarkRealFrame, about 15KB). Chunked, since kitty caps one escape's
// payload at MaxChunkSize.
func (m *Model) markTransmit() string {
	// Tint first, then check markSent: a `t` re-tints and clears markSent, and
	// the other order would leave the old colour on screen.
	img := m.markImage()
	if img == nil || m.markSent {
		return ""
	}
	var buf bytes.Buffer
	err := ansi.WriteKittyGraphics(&buf, img, &kitty.Options{
		Action:       kitty.Transmit,
		ID:           markImageID,
		Format:       kitty.PNG,
		Transmission: kitty.Direct,
		Chunk:        true,
		Quite:        2, // no reply, which would otherwise arrive as key presses
	})
	if err != nil {
		return ""
	}
	m.markSent = true
	return buf.String()
}

// markPlaces are the sequences that draw the picture at the cursor, one per
// placement id. They are a few dozen bytes, cheap on a line that repaints, and
// built once. C=1 keeps the cursor still: the buffer counts one column per
// rune, so a moving cursor would shift the rest of the row.
var markPlaces = sync.OnceValue(func() []string {
	out := make([]string, markPlacements)
	for i := range out {
		out[i] = ansi.KittyGraphics(nil, (&kitty.Options{
			Action:          kitty.Put,
			ID:              markImageID,
			PlacementID:     i + 1,
			Columns:         MarkW,
			Rows:            MarkH,
			DoNotMoveCursor: true,
			Quite:           2,
		}).Options()...)
	}
	return out
})

// markDeletes are the sequences that take each placement off the screen.
// kitty keeps placements in their own layer, so the line and screen erases
// Bubble Tea repaints with leave the image where it was.
var markDeletes = sync.OnceValue(func() []string {
	out := make([]string, markPlacements)
	for i := range out {
		out[i] = ansi.KittyGraphics(nil, (&kitty.Options{
			Action:      kitty.Delete,
			Delete:      kitty.DeleteID,
			ID:          markImageID,
			PlacementID: i + 1,
			Quite:       2,
		}).Options()...)
	}
	return out
})
