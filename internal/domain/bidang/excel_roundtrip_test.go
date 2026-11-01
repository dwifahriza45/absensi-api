package bidang

import (
	"bytes"
	"testing"
)

func TestExportCanBeImportedAgain(t *testing.T) {
	data, err := writeExcel([]Response{{Name: "Nama Uji"}})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := readExcel(bytes.NewReader(data))
	if err != nil || len(rows) != 1 || rows[0].Name != "Nama Uji" {
		t.Fatalf("export/import mismatch: %+v %v", rows, err)
	}
}
