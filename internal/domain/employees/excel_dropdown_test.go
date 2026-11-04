package employees

import (
	"bytes"
	"github.com/xuri/excelize/v2"
	"testing"
)

func TestEmployeeWorkbookDropdowns(t *testing.T) {
	options := FilterOptions{Mitra: []string{"Mitra, dengan koma", "Mitra lain"}, Bidang: []string{"Bidang A", "Bidang B"}}
	for _, items := range [][]Response{nil, {{NIK: "0123456789012345", EmployeeID: "EMP-20261103-000001", Name: "Karyawan", Mitra: options.Mitra[0], Bidang: options.Bidang[0]}}} {
		data, err := employeeWorkbook(items, options)
		if err != nil {
			t.Fatal(err)
		}
		f, err := excelize.OpenReader(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		rows, err := f.GetRows("Karyawan")
		if err != nil || len(rows[0]) != 5 {
			t.Fatal("wrong workbook columns", err)
		}
		for i, want := range []string{"NAMA LENGKAP", "MITRA", "BIDANG", "NIK", "ID KARYAWAN"} {
			column, _ := excelize.ColumnNumberToName(i + 1)
			visible, err := f.GetColVisible("Karyawan", column)
			if err != nil || visible != (i < 4) || rows[0][i] != want {
				t.Fatalf("column %s: header=%q visible=%v err=%v", column, rows[0][i], visible, err)
			}
		}
		validations, err := f.GetDataValidations("Karyawan")
		if err != nil || len(validations) != 2 {
			t.Fatal("missing master dropdowns", err)
		}
		for i, want := range []struct{ rangeName, sqref string }{{"PilihanMitra", "B2:B5001"}, {"PilihanBidang", "C2:C5001"}} {
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
		for _, cell := range []string{"D2", "D100", "D5001", "E2", "E100", "E5001"} {
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
			id, err := f.GetCellValue("Karyawan", "E2")
			if err != nil || id != items[0].EmployeeID {
				t.Fatal("hidden employee ID must be preserved for re-import", err)
			}
			nik, err := f.GetCellValue("Karyawan", "D2")
			if err != nil || nik != "0123456789012345" {
				t.Fatal("NIK digits changed", err)
			}
			kind, err := f.GetCellType("Karyawan", "D2")
			if err != nil || kind != excelize.CellTypeSharedString {
				t.Fatal("NIK must be stored as text", err)
			}

		}
		f.Close()
	}
}
