package app

import (
	"bytes"
	"context"
	"errors"
	"github.com/haidarrasyad/absensi-api/internal/domain/attendances"
	"github.com/haidarrasyad/absensi-api/internal/pkg/database"
	"testing"
)

func TestCheckoutRequiresReportsOrExistingEarlyLeave(t *testing.T) {
	db, _ := concurrencyDB(t)
	ctx := context.Background()
	id := newEmployee(t, db)
	svc, _, _ := services(t, db, newTestStore(t), database.NewCoordinator(db))
	location := &attendances.Location{Lat: -6.9, Lng: 106.9, AccuracyM: 10}
	if _, err := svc.CheckIn(ctx, id, attendances.CheckInInput{Selfie: bytes.NewReader(testPNG), Location: location}); err != nil {
		t.Fatal(err)
	}
	for _, reason := range []string{"", "Sakit", "Waktu tidak cukup"} {
		_, err := svc.CheckOut(ctx, id, attendances.CheckOutInput{Selfie: bytes.NewReader(testPNG), Location: location, Reason: reason})
		if !errors.Is(err, attendances.ErrReportsIncomplete) {
			t.Fatalf("reason bypassed missing reports: %v", err)
		}
	}
	if rowCount(t, db, "photos") != 1 {
		t.Fatal("blocked checkout saved a selfie")
	}
	if _, err := svc.RequestEarlyLeave(ctx, id, attendances.EarlyLeaveRequest{Reason: "Sakit"}); err != nil {
		t.Fatal(err)
	}
	result, err := svc.CheckOut(ctx, id, attendances.CheckOutInput{Selfie: bytes.NewReader(testPNG), Location: location, Reason: "Override"})
	if err != nil || result.CheckOutAt == nil || result.CheckOutReason == nil || *result.CheckOutReason != "Sakit" {
		t.Fatalf("early leave not preserved: %+v %v", result, err)
	}
}
