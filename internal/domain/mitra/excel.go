package mitra

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/xuri/excelize/v2"
)

const maxExcelFile = 8 << 20
const maxExcelExpanded = 32 << 20

var errExcel = errors.New("File Excel tidak valid. Gunakan template .xlsx dengan sheet Mitra dan satu kolom NAMA MITRA.")

func writeExcel(items []Response) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()
	if err := f.SetSheetName("Sheet1", "Mitra"); err != nil {
		return nil, err
	}
	if err := f.SetCellStr("Mitra", "A1", "NAMA MITRA"); err != nil {
		return nil, err
	}

	headerStyle, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF", Size: 11},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"0D365B"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	if err != nil {
		return nil, err
	}
	if err = f.SetCellStyle("Mitra", "A1", "A1", headerStyle); err != nil {
		return nil, err
	}
	if err = f.SetRowHeight("Mitra", 1, 26); err != nil {
		return nil, err
	}
	for i, item := range items {
		// String cells prevent names from becoming spreadsheet formulas.
		if err := f.SetCellStr("Mitra", fmt.Sprintf("A%d", i+2), item.Name); err != nil {
			return nil, err
		}
	}
	if err := f.SetColWidth("Mitra", "A", "A", 65); err != nil {
		return nil, err
	}

	buffer, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
func readExcel(reader io.Reader) ([]ImportRow, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxExcelFile+1))
	if err != nil {
		return nil, errExcel
	}
	if len(data) > maxExcelFile {
		return nil, errors.New("File Excel maksimal 8 MiB.")
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, errExcel
	}
	if len(z.File) > 1024 {
		return nil, errExcel
	}
	var expanded uint64
	for _, file := range z.File {
		if file.UncompressedSize64 > maxExcelExpanded-expanded {
			return nil, errExcel
		}
		expanded += file.UncompressedSize64
		name := strings.ToLower(file.Name)
		if strings.Contains(name, "vbaproject") || strings.HasPrefix(name, "xl/externallinks/") {
			return nil, errExcel
		}
	}
	f, err := excelize.OpenReader(bytes.NewReader(data), excelize.Options{UnzipSizeLimit: maxExcelExpanded, UnzipXMLSizeLimit: maxExcelExpanded})
	if err != nil {
		return nil, errExcel
	}
	defer f.Close()
	rows, err := f.Rows("Mitra")
	if err != nil {
		return nil, errExcel
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, errExcel
	}
	header, err := rows.Columns()
	if err != nil {
		return nil, errExcel
	}
	nameColumn, idColumn := 0, -1
	switch {
	case len(header) == 1 && validHeader(header[0], "NAMA MITRA", "name"):
	case len(header) == 2 && validHeader(header[0], "Kode Mitra", "mitra_id") && validHeader(header[1], "Nama Mitra", "name"):
		idColumn, nameColumn = 0, 1 // Accept previously downloaded files.
	default:
		return nil, errExcel
	}

	result := make([]ImportRow, 0)
	for line := 2; rows.Next(); line++ {
		// Count physical rows, including blanks, to cap sparse malicious worksheets.
		if line > maxImportRows+1 {
			return nil, errors.New("Maksimal 5.000 baris per import.")
		}
		cells, err := rows.Columns()
		if err != nil {
			return nil, errExcel
		}
		item := ImportRow{Row: line}
		if idColumn >= 0 && len(cells) > idColumn {
			item.MitraID = cells[idColumn]
		}
		if len(cells) > nameColumn {
			item.Name = cells[nameColumn]
		}

		for col := 1; col <= len(header); col++ {
			axis, _ := excelize.CoordinatesToCellName(col, line)
			formula, e := f.GetCellFormula("Mitra", axis)
			if e != nil {
				return nil, errExcel
			}
			if formula != "" {
				item.Problem = "Formula tidak diizinkan pada data import."
			}
		}
		if len(cells) > len(header) {
			item.Problem = "Baris memiliki kolom tambahan di luar template."
		}
		// Keep blank rows as errors so each row has a reported outcome.
		result = append(result, item)
	}
	if err := rows.Error(); err != nil {
		return nil, errExcel
	}
	if len(result) == 0 {
		return nil, errors.New("File Excel belum berisi data mitra.")
	}
	return result, nil
}

func validHeader(value string, labels ...string) bool {
	value = strings.TrimSpace(value)
	for _, label := range labels {
		if strings.EqualFold(value, label) {
			return true
		}
	}
	return false
}
