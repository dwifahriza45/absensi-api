// Package pdfreport writes PDF objects directly to the response, retaining only
// one page's images and a small cross-reference table, not the complete document.
package pdfreport

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png"
	"io"
	"strings"
)

//go:embed logo.png
var logo []byte

type Writer struct {
	out     io.Writer
	offset  int64
	offsets []int64
	pages   []int
	err     error
	Logo    int
}
type Picture struct {
	ID            int
	Width, Height int
}

func New(out io.Writer) *Writer {
	w := &Writer{out: out, offsets: []int64{0, 0, 0, 0, 0}}
	w.write("%PDF-1.4\n% report\n")
	w.object(1, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
	w.object(2, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>")
	p, err := w.Image(bytes.NewReader(logo))
	if err != nil {
		w.err = err
	}
	w.Logo = p.ID
	return w
}
func (w *Writer) write(s string) {
	if w.err != nil {
		return
	}
	n, err := io.WriteString(w.out, s)
	w.offset += int64(n)
	w.err = err
}
func (w *Writer) bytes(p []byte) {
	if w.err != nil {
		return
	}
	n, err := w.out.Write(p)
	w.offset += int64(n)
	w.err = err
}
func (w *Writer) alloc() int { w.offsets = append(w.offsets, 0); return len(w.offsets) - 1 }
func (w *Writer) object(id int, s string) {
	w.offsets[id] = w.offset
	w.write(fmt.Sprintf("%d 0 obj\n%s\nendobj\n", id, s))
}
func (w *Writer) stream(id int, attrs string, data []byte) {
	w.offsets[id] = w.offset
	w.write(fmt.Sprintf("%d 0 obj\n<< %s /Length %d >>\nstream\n", id, attrs, len(data)))
	w.bytes(data)
	w.write("\nendstream\nendobj\n")
}
func (w *Writer) Image(r io.Reader) (Picture, error) {
	raw, err := io.ReadAll(io.LimitReader(r, (8<<20)+1))
	if err != nil || len(raw) > 8<<20 {
		return Picture{}, fmt.Errorf("invalid photo size")
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || config.Width < 1 || config.Height < 1 || int64(config.Width)*int64(config.Height) > 25000000 {
		return Picture{}, fmt.Errorf("invalid photo dimensions")
	}
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return Picture{}, err
	}
	b := src.Bounds()
	width, height := b.Dx(), b.Dy()
	if width < 1 || height < 1 {
		return Picture{}, fmt.Errorf("invalid image")
	}
	// Embedded report photos are bounded to 640 px; originals stay in private storage.
	nw, nh := width, height
	if width > 640 || height > 640 {
		if width >= height {
			nw = 640
			nh = height * 640 / width
		} else {
			nh = 640
			nw = width * 640 / height
		}
	}
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		for x := 0; x < nw; x++ {
			r, g, b, a := src.At(b.Min.X+x*width/nw, b.Min.Y+y*height/nh).RGBA()
			dst.SetRGBA(x, y, color.RGBA{R: uint8((r + 65535 - a) >> 8), G: uint8((g + 65535 - a) >> 8), B: uint8((b + 65535 - a) >> 8), A: 255})
		}
	}
	var data bytes.Buffer
	if err = jpeg.Encode(&data, dst, &jpeg.Options{Quality: 70}); err != nil {
		return Picture{}, err
	}
	id := w.alloc()
	w.stream(id, fmt.Sprintf("/Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode", nw, nh), data.Bytes())
	return Picture{ID: id, Width: nw, Height: nh}, w.err
}
func escape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\\', '(', ')':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			if r >= 32 && r <= 255 {
				b.WriteByte(byte(r))
			} else if r == '\n' || r == '\r' {
				b.WriteByte(' ')
			} else {
				b.WriteByte('-')
			}
		}
	}
	return b.String()
}

type Page struct {
	content  strings.Builder
	pictures []int
}

func (p *Page) Text(x, y, size float64, bold bool, s string) {
	font := "F1"
	if bold {
		font = "F2"
	}
	fmt.Fprintf(&p.content, "BT /%s %.1f Tf 1 0 0 1 %.1f %.1f Tm (%s) Tj ET\n", font, size, x, 842-y, escape(s))
}
func (p *Page) Color(r, g, b float64) { fmt.Fprintf(&p.content, "%.3f %.3f %.3f rg\n", r, g, b) }
func (p *Page) Rect(x, y, width, height float64) {
	fmt.Fprintf(&p.content, "%.1f %.1f %.1f %.1f re f\n", x, 842-y-height, width, height)
}
func (p *Page) Image(pic Picture, x, y, width, height float64) {
	if pic.ID == 0 {
		return
	}
	scale := width / float64(pic.Width)
	if float64(pic.Height)*scale > height {
		scale = height / float64(pic.Height)
	}
	pw, ph := float64(pic.Width)*scale, float64(pic.Height)*scale
	p.pictures = append(p.pictures, pic.ID)
	fmt.Fprintf(&p.content, "q %.1f 0 0 %.1f %.1f %.1f cm /I%d Do Q\n", pw, ph, x+(width-pw)/2, 842-y-(height+ph)/2, pic.ID)
}
func (w *Writer) Page(p *Page) error {
	content := w.alloc()
	w.stream(content, "", []byte(p.content.String()))
	id := w.alloc()
	var resources strings.Builder
	for _, pic := range p.pictures {
		fmt.Fprintf(&resources, "/I%d %d 0 R ", pic, pic)
	}
	w.object(id, fmt.Sprintf("<< /Type /Page /Parent 3 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 1 0 R /F2 2 0 R >> /XObject << %s >> >> /Contents %d 0 R >>", resources.String(), content))
	w.pages = append(w.pages, id)
	return w.err
}
func (w *Writer) Close() error {
	var kids strings.Builder
	for _, id := range w.pages {
		fmt.Fprintf(&kids, "%d 0 R ", id)
	}
	w.object(3, fmt.Sprintf("<< /Type /Pages /Count %d /Kids [%s] >>", len(w.pages), kids.String()))
	w.object(4, "<< /Type /Catalog /Pages 3 0 R >>")
	start := w.offset
	w.write(fmt.Sprintf("xref\n0 %d\n0000000000 65535 f \n", len(w.offsets)))
	for _, offset := range w.offsets[1:] {
		w.write(fmt.Sprintf("%010d 00000 n \n", offset))
	}
	w.write(fmt.Sprintf("trailer\n<< /Size %d /Root 4 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(w.offsets), start))
	return w.err
}
