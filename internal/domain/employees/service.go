package employees

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/haidarrasyad/absensi-api/internal/shared"
)

const (
	searchLimit     = 10
	minSearchLength = 3
	maxImportRows   = 5000
)

// Service berisi aturan bisnis pegawai.
type Service struct {
	repo Repository
}

// NewService membuat Service pegawai.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Search mencari nama pegawai aktif untuk layar login (maks 10 hasil).
func (s *Service) Search(ctx context.Context, query string) ([]SearchItem, error) {
	query = strings.TrimSpace(query)
	if utf8.RuneCountInString(query) < minSearchLength {
		return nil, ErrQueryTooShort
	}

	list, err := s.repo.Search(ctx, query, searchLimit)
	if err != nil {
		return nil, err
	}

	items := make([]SearchItem, len(list))
	for i, e := range list {
		items[i] = SearchItem{ID: e.ID, EmployeeID: e.EmployeeID, Name: e.Name, MitraID: e.MitraID, Mitra: e.Mitra, Bidang: e.Bidang, BidangID: e.BidangID}
	}
	return items, nil
}

// GetByID mengambil pegawai berdasarkan employee_id (EMP), bukan UUID.
func (s *Service) GetByID(ctx context.Context, id string) (Employee, error) {
	if !shared.IsPublicID(id, "EMP") {
		return Employee{}, ErrNotFound
	}
	return s.repo.FindByID(ctx, id)
}

// List mengembalikan pegawai aktif sesuai filter.
func (s *Service) List(ctx context.Context, filter ListFilter) ([]Response, error) {
	list, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	items := make([]Response, len(list))
	for i, e := range list {
		items[i], err = s.adminResponse(e)
		if err != nil {
			return nil, err
		}
	}
	return items, nil
}

// FilterOptions mengembalikan pilihan mitra dan bidang.
func (s *Service) FilterOptions(ctx context.Context) (FilterOptions, error) {
	return s.repo.FilterOptions(ctx)
}

// Import validates Excel rows and matches employees by ID or NIK.
func (s *Service) Import(ctx context.Context, req ImportRequest) (ImportResult, error) {
	if len(req.Rows) > maxImportRows {
		return ImportResult{}, ErrImportTooLarge
	}

	valid := make([]Employee, 0, len(req.Rows))
	skipped := make([]SkippedRow, 0)
	seenNIK := make(map[string]bool, len(req.Rows))
	seenIDs := make(map[string]bool, len(req.Rows))

	for i, row := range req.Rows {
		rowNumber := i + 2 // baris 1 di Excel adalah judul kolom
		name := clean(row.Name, 80)

		if name == "" {
			skipped = append(skipped, SkippedRow{Row: rowNumber, Name: "(tanpa nama)", Reason: "Nama kosong"})
			continue
		}
		nik := strings.TrimSpace(row.NIK)
		if !shared.ValidNIK(nik) {
			skipped = append(skipped, SkippedRow{Row: rowNumber, Name: name, Reason: "NIK Harus 16 Digit Angka"})
			continue
		}
		employeeID := strings.TrimSpace(row.EmployeeID)
		if employeeID != "" && !shared.IsPublicID(employeeID, "EMP") {
			skipped = append(skipped, SkippedRow{Row: rowNumber, Name: name, Reason: "ID Karyawan Tidak Valid"})
			continue
		}
		if seenNIK[nik] || employeeID != "" && seenIDs[employeeID] {
			skipped = append(skipped, SkippedRow{Row: rowNumber, Name: name, Reason: "NIK atau ID karyawan sama dengan baris lain"})
			continue
		}

		if strings.TrimSpace(row.Bidang) == "" {
			skipped = append(skipped, SkippedRow{Row: rowNumber, Name: name, Reason: "Bidang Wajib Diisi"})
			continue
		}

		seenNIK[nik] = true
		if employeeID != "" {
			seenIDs[employeeID] = true
		}

		valid = append(valid, Employee{
			EmployeeID: employeeID,
			NIK:        nik,

			Name:  name,
			Mitra: clean(row.Mitra, 80),

			Bidang: clean(row.Bidang, 60),
		})
	}

	if len(valid) == 0 {
		return ImportResult{Skipped: skipped}, ErrImportEmpty
	}

	result, err := s.repo.Import(ctx, valid, req.DeactivateMissing)
	if err != nil {
		return ImportResult{}, err
	}
	result.Skipped = skipped
	return result, nil
}

// clean merapikan spasi dan membatasi panjang teks.
func clean(value string, maxRunes int) string {
	value = strings.Join(strings.Fields(value), " ")
	if utf8.RuneCountInString(value) > maxRunes {
		value = string([]rune(value)[:maxRunes])
	}
	return value
}

func (s *Service) GetByName(ctx context.Context, name string) (Employee, error) {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		return Employee{}, ErrNotFound
	}
	return s.repo.FindByName(ctx, name)
}

func (s *Service) GetByNIK(ctx context.Context, nik string) (Employee, error) {
	return s.repo.FindByNIK(ctx, strings.TrimSpace(nik))
}
