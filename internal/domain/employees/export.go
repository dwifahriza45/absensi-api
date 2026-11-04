package employees

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/haidarrasyad/absensi-api/internal/pkg/apperror"
	"github.com/haidarrasyad/absensi-api/internal/pkg/httpx"
	"github.com/xuri/excelize/v2"
)

// Export applies the same name/NIK search and status filter as the UI.
func (h *Handler) Export(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	status := q.Get("status")
	if status == "" {
		status = "active"
	}
	if status != "active" && status != "inactive" && status != "all" {
		httpx.Fail(w, r, apperror.BadRequest("invalid_status", "Status karyawan tidak valid."))
		return
	}
	items, err := h.svc.List(r.Context(), ListFilter{MitraID: q.Get("mitra_id"), BidangID: q.Get("bidang_id"), IncludeInactive: true})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	needle := strings.ToLower(strings.TrimSpace(q.Get("q")))
	filtered := make([]Response, 0, len(items))
	for _, item := range items {
		if status != "all" && item.IsActive != (status == "active") {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(item.Name+" "+item.NIK), needle) {
			continue
		}
		filtered = append(filtered, item)
	}
	options, err := h.svc.FilterOptions(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	data, err := employeeWorkbook(filtered, options)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="karyawan.xlsx"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
func employeeWorkbook(items []Response, options FilterOptions) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()
	if err := f.SetSheetName("Sheet1", "Karyawan"); err != nil {
		return nil, err
	}
	style, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Color: "FFFFFF", Size: 11}, Fill: excelize.Fill{Type: "pattern", Color: []string{"0D365B"}, Pattern: 1}, Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"}})
	if err != nil {
		return nil, err
	}
	// Format NIK and the hidden identifier as text, including blank rows.
	textStyle, err := f.NewStyle(&excelize.Style{NumFmt: 49})
	if err != nil {
		return nil, err
	}
	if err = f.SetColStyle("Karyawan", "D:E", textStyle); err != nil {
		return nil, err
	}
	headers := []string{"NAMA LENGKAP", "MITRA", "BIDANG", "NIK", "ID KARYAWAN"}
	for i, value := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err = f.SetCellStr("Karyawan", cell, value); err != nil {
			return nil, err
		}
	}
	if err = f.SetCellStyle("Karyawan", "A1", "E1", style); err != nil {
		return nil, err
	}
	if err = f.SetRowHeight("Karyawan", 1, 26); err != nil {
		return nil, err
	}
	if err = f.SetColWidth("Karyawan", "A", "E", 26); err != nil {
		return nil, err
	}
	// Keep the identifier for re-importing edits, but hide it in both downloads.
	if err = f.SetColVisible("Karyawan", "E", false); err != nil {
		return nil, err
	}
	for i, item := range items {
		for j, value := range []string{item.Name, item.Mitra, item.Bidang, item.NIK, item.EmployeeID} {
			cell, _ := excelize.CoordinatesToCellName(j+1, i+2)
			// Explicit strings preserve leading zeroes and never execute formulas.
			if err = f.SetCellStr("Karyawan", cell, value); err != nil {
				return nil, err
			}
		}
	}
	if err := employeeDropdowns(f, options); err != nil {
		return nil, err
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Template uses the same columns, styles, and visibility as employee exports.
func (h *Handler) Template(w http.ResponseWriter, r *http.Request) {
	options, err := h.svc.FilterOptions(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	data, err := employeeWorkbook(nil, options)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="template_data_pegawai.xlsx"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// Named ranges keep dropdowns working across sheets and avoid Excel's 255-character inline list limit.
func employeeDropdowns(f *excelize.File, options FilterOptions) error {
	const sheet = "Referensi"
	if _, err := f.NewSheet(sheet); err != nil {
		return err
	}
	for _, entry := range []struct {
		column, target, name string
		values               []string
	}{
		{"A", "B", "PilihanMitra", options.Mitra},
		{"B", "C", "PilihanBidang", options.Bidang},
	} {
		if len(entry.values) == 0 {
			continue
		}
		for i, value := range entry.values {
			if err := f.SetCellStr(sheet, fmt.Sprintf("%s%d", entry.column, i+1), value); err != nil {
				return err
			}
		}
		if err := f.SetDefinedName(&excelize.DefinedName{Name: entry.name, RefersTo: fmt.Sprintf("'%s'!$%s$1:$%s$%d", sheet, entry.column, entry.column, len(entry.values))}); err != nil {
			return err
		}
		validation := excelize.NewDataValidation(false)
		validation.SetSqref(fmt.Sprintf("%s2:%s5001", entry.target, entry.target))
		validation.SetSqrefDropList(entry.name)
		validation.SetError(excelize.DataValidationErrorStyleStop, "Pilihan tidak valid", "Pilih nama dari dropdown data master.")
		if err := f.AddDataValidation("Karyawan", validation); err != nil {
			return err
		}
	}
	return f.SetSheetVisible(sheet, false)
}
