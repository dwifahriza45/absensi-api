package employees

import (
	"context"
	"github.com/haidarrasyad/absensi-api/internal/pkg/validation"
	"testing"
)

func TestNIKValidationAndDuplicateImport(t *testing.T) {
	for _, nik := range []string{"", "123", "12345678901234567", "123456789012345a", "123456789012345 ", "１２３４５６７８９０１２３４５６"} {
		req := WriteRequest{Name: "Test", NIK: nik, MitraID: "MTR-20261103-000001", BidangID: "BID-20261103-000001"}
		if err := validation.Struct(&req); err == nil {
			t.Fatalf("invalid NIK accepted: %q", nik)
		}
	}
	repo := &importCapture{}
	svc := NewService(repo)
	rows := []ImportRow{
		{Name: "First", NIK: "0123456789012345", Bidang: "IT"},
		{Name: "Duplicate", NIK: "0123456789012345", Bidang: "IT"},
		{Name: "Invalid", NIK: "123", Bidang: "IT"},
	}
	result, err := svc.Import(context.Background(), ImportRequest{Rows: rows})
	if err != nil || result.Created != 1 || len(result.Skipped) != 2 || repo.rows[0].NIK != rows[0].NIK {
		t.Fatalf("invalid import result: %+v %v", result, err)
	}
}
