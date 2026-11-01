package pdfreport

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"testing"
)

func TestPDFCrossReferencesAndEscapedText(t *testing.T) {
	var out bytes.Buffer
	w := New(&out)
	for i := 0; i < 3; i++ {
		var p Page
		p.Text(36, 40, 10, false, `Employee (name) \ test`)
		if err := w.Page(&p); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	data := out.Bytes()
	for id, offset := range w.offsets[1:] {
		want := fmt.Sprintf("%d 0 obj", id+1)
		if !bytes.HasPrefix(data[offset:], []byte(want)) {
			t.Fatalf("incorrect xref for %d", id+1)
		}
	}
	matches := regexp.MustCompile(`startxref\n([0-9]+)`).FindSubmatch(data)
	if len(matches) != 2 {
		t.Fatal("missing xref")
	}
	offset, _ := strconv.Atoi(string(matches[1]))
	if !bytes.HasPrefix(data[offset:], []byte("xref")) {
		t.Fatal("invalid xref offset")
	}
	if !bytes.Contains(data, []byte(`Employee \(name\) \\ test`)) {
		t.Fatal("PDF text was not escaped")
	}
}
