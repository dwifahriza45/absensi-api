package app

import (
	"bytes"
	"context"
	"errors"
	"github.com/haidarrasyad/absensi-api/internal/domain/attendances"
	"github.com/haidarrasyad/absensi-api/internal/pkg/database"
	"testing"
)

func TestRequiredSelfieLocationAndFlexibleReportHours(t *testing.T) {
	db, _ := concurrencyDB(t)
	ctx := context.Background()
	id := newEmployee(t, db)
	a, reports, _ := services(t, db, newTestStore(t), database.NewCoordinator(db))
	location := &attendances.Location{Lat: -6.9, Lng: 106.9, AccuracyM: 10}
	if _, err := a.CheckIn(ctx, id, attendances.CheckInInput{Location: location}); !errors.Is(err, attendances.ErrSelfieRequired) {
		t.Fatalf("missing selfie accepted: %v", err)
	}
	if _, err := a.CheckIn(ctx, id, attendances.CheckInInput{Selfie: bytes.NewReader(testPNG)}); !errors.Is(err, attendances.ErrLocationRequired) {
		t.Fatalf("missing location accepted: %v", err)
	}
	if _, err := a.CheckIn(ctx, id, attendances.CheckInInput{Selfie: bytes.NewReader(testPNG), Location: location}); err != nil {
		t.Fatal(err)
	}
	// At 10:00, session 3 is still upcoming. Upload is allowed without waiting.
	if _, err := reports.Submit(ctx, id, 3, bytes.NewReader(testPNG)); err != nil {
		t.Fatalf("session time gate still enforced: %v", err)
	}
	if _, err := a.CheckOut(ctx, id, attendances.CheckOutInput{Location: location}); !errors.Is(err, attendances.ErrSelfieRequired) {
		t.Fatalf("missing checkout selfie accepted: %v", err)
	}
	if _, err := a.CheckOut(ctx, id, attendances.CheckOutInput{Selfie: bytes.NewReader(testPNG)}); !errors.Is(err, attendances.ErrLocationRequired) {
		t.Fatalf("missing checkout location accepted: %v", err)
	}
}
