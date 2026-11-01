package employees

import (
	"bytes"
	"github.com/xuri/excelize/v2"
	"testing"
)

func TestEmployeeWorkbookDropdowns(t *testing.T) {
	options := FilterOptions{Mitra: []string{"Mitra, dengan koma", "Mitra lain"}, Bidang: []string{"Bidang A", "Bidang B"}}
	for _, items := range [][]Response{nil, {{Name: "Karyawan", Phone: "6281234567890", Mitra: options.Mitra[0], Bidang: options.Bidang[0]}}} {
		data, err := employeeWorkbook(items, options)
		if err != nil {
			t.Fatal(err)
		}
		f, err := excelize.OpenReader(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		rows, err := f.GetRows("Karyawan")
		if err != nil || len(rows[0]) != 4 {
			t.Fatal("wrong workbook columns", err)
		}
		validations, err := f.GetDataValidations("Karyawan")
		if err != nil || len(validations) != 2 {
			t.Fatal("missing master dropdowns", err)
		}
		for i, want := range []struct{ rangeName, sqref string }{{"PilihanMitra", "C2:C5001"}, {"PilihanBidang", "D2:D5001"}} {
			if validations[i].Formula1 != want.rangeName || validations[i].Sqref != want.sqref {
				t.Fatalf("incorrect validation: %+v", validations[i])
			}
		}
		visible, err := f.GetSheetVisible("Referensi")
		if err != nil || visible {
			t.Fatal("reference sheet must be hidden")
		}
		value, err := f.GetCellValue("Referensi", "A1")
		if err != nil || value != options.Mitra[0] {
			t.Fatal("master names not preserved")
		}
		if len(f.GetDefinedName()) != 2 {
			t.Fatal("missing named ranges")
		}
		for _, cell := range []string{"B2", "B100", "B5001"} {
			styleID, err := f.GetCellStyle("Karyawan", cell)
			if err != nil {
				t.Fatal(err)
			}
			style, err := f.GetStyle(styleID)
			if err != nil || style.NumFmt != 49 {
				t.Fatalf("%s must use Text format: %+v %v", cell, style, err)
			}
		}
		if len(items) > 0 {
			value, err := f.GetCellValue("Karyawan", "B2")
			if err != nil || value != "6281234567890" {
				t.Fatal("phone digits changed", err)
			}
			cellType, err := f.GetCellType("Karyawan", "B2")
			if err != nil || cellType != excelize.CellTypeSharedString {
				t.Fatal("phone must be stored as text", err)
			}
		}
		f.Close()
	}
}
