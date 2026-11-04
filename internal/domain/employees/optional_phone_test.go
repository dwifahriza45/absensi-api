package employees

import (
	"context"
	"github.com/haidarrasyad/absensi-api/internal/shared"
	"testing"
)

type optionalPhoneRepo struct{ importCapture }

func (r *optionalPhoneRepo) Save(_ context.Context, _ string, e Employee, _ *bool) (Employee, error) {
	r.rows = append(r.rows, e)
	return e, nil
}
func TestSaveOptionalAndNormalizedPhone(t *testing.T) {
	repo := &optionalPhoneRepo{}
	svc := NewService(repo, shared.NewPhoneHasher("optional-phone"))
	for _, phone := range []string{"", "  ", "081234567890", "+6281234567890", "6281234567890"} {
		result, err := svc.Save(context.Background(), "", WriteRequest{NIK: "0123456789012345", Name: "Employee", MitraID: "MTR-20261103-000001", BidangID: "BID-20261103-000001", Phone: phone})
		if err != nil {
			t.Fatal(err)
		}
		want := shared.NormalizePhone(phone)
		if result.Phone != want {
			t.Fatalf("phone=%q want=%q", result.Phone, want)
		}
		saved := repo.rows[len(repo.rows)-1]
		if want == "" && (saved.PhoneHash != "" || saved.PhoneEncrypted != "") {
			t.Fatal("blank contact must not be hashed or encrypted")
		}
	}
}
func TestImportMultipleBlankPhonesAndNormalizedContacts(t *testing.T) {
	repo := &importCapture{}
	hasher := shared.NewPhoneHasher("optional-phone")
	svc := NewService(repo, hasher)
	result, err := svc.Import(context.Background(), ImportRequest{Rows: []ImportRow{
		{Name: "No contact one", NIK: "0123456789012345", Bidang: "IT"},
		{Name: "No contact two", NIK: "0123456789012346", Bidang: "IT", Phone: "  "},
		{Name: "Local contact", NIK: "0123456789012347", Bidang: "IT", Phone: "081234567890"},
		{Name: "International contact", NIK: "0123456789012348", Bidang: "IT", Phone: "+6281234567891"},
		{Name: "Duplicate contact", NIK: "0123456789012349", Bidang: "IT", Phone: "+6281234567890"},
	}})
	if err != nil || result.Created != 4 || len(result.Skipped) != 1 {
		t.Fatalf("%+v %v", result, err)
	}
	for i, want := range []string{"", "", "6281234567890", "6281234567891"} {
		row := repo.rows[i]
		got, err := hasher.Decrypt(row.PhoneEncrypted, row.PhoneHash)
		if err != nil || got != want {
			t.Fatalf("row %d: phone=%q err=%v", i, got, err)
		}
		if want == "" && row.PhoneHash != "" {
			t.Fatal("blank phone must have no lookup hash")
		}
	}
}
