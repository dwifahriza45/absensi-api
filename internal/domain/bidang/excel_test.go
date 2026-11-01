package bidang

import (
	"archive/zip"
	"bytes"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestExcelRoundTripUsesStringCells(t *testing.T) {
	input := []Response{{BidangID: "BID-20261031-000001", Name: "=HYPERLINK(\"https://example.invalid\")"}}
	data, err := writeExcel(input)
	if err != nil {
		t.Fatal(err)
	}
	book, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	sheetRows, err := book.GetRows("Bidang")
	book.Close()
	if err != nil || len(sheetRows) < 2 || len(sheetRows[0]) != 1 || sheetRows[0][0] != "NAMA BIDANG" || len(sheetRows[1]) != 1 {
		t.Fatal("export must contain only human-readable name column")
	}
	rows, err := readExcel(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Name != input[0].Name || rows[0].Problem != "" {
		t.Fatalf("unsafe string handling: %+v", rows)
	}
}
func TestExcelRejectsInvalidFilesAndMarksFormulas(t *testing.T) {
	if _, err := readExcel(bytes.NewReader([]byte("not an xlsx"))); err == nil {
		t.Fatal("accepted non-Excel")
	}
	f := excelize.NewFile()
	defer f.Close()
	f.SetSheetName("Sheet1", "Bidang")
	f.SetCellStr("Bidang", "A1", "bidang_id")
	f.SetCellStr("Bidang", "B1", "name")
	f.SetCellFormula("Bidang", "B2", "1+1")
	data, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	rows, err := readExcel(bytes.NewReader(data.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Problem == "" {
		t.Fatal("formula not flagged")
	}
	f.SetCellStr("Bidang", "A1", "wrong_header")
	data, err = f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = readExcel(bytes.NewReader(data.Bytes())); err == nil {
		t.Fatal("accepted invalid header")
	}
}

func TestExcelRejectsOversizedArchiveBeforeDecompression(t *testing.T) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	part, err := writer.CreateRaw(&zip.FileHeader{Name: "xl/worksheets/sheet1.xml", Method: zip.Store, CompressedSize64: 1, UncompressedSize64: maxExcelExpanded + 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = part.Write([]byte{0}); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = readExcel(bytes.NewReader(buffer.Bytes())); err == nil {
		t.Fatal("accepted oversized expanded archive")
	}
}
