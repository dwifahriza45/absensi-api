package employees

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type employeeCapture struct{ importCapture }

func (r *employeeCapture) Save(_ context.Context, _ string, e Employee, _ *bool) (Employee, error) {
	r.rows = append(r.rows, e)
	return e, nil
}
func TestSaveAndImportWithoutPhone(t *testing.T) {
	repo := &employeeCapture{}
	svc := NewService(repo)
	result, err := svc.Save(context.Background(), "", WriteRequest{NIK: "0123456789012345", Name: "Employee", MitraID: "MTR-20261103-000001", BidangID: "BID-20261103-000001"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(result)
	if err != nil || strings.Contains(string(data), "phone") {
		t.Fatalf("unexpected contact in response: %s %v", data, err)
	}
	imported, err := svc.Import(context.Background(), ImportRequest{Rows: []ImportRow{
		{Name: "First", NIK: "0123456789012345", Bidang: "IT"},
		{Name: "Second", NIK: "0123456789012346", Bidang: "IT"},
	}})
	if err != nil || imported.Created != 2 || len(imported.Skipped) != 0 {
		t.Fatalf("%+v %v", imported, err)
	}
}
