package workreports

import "time"

// SubmitResponse adalah hasil kirim foto hasil kerja.
// Duplicate bernilai true bila sesi ini sudah pernah dikirim (kirim ulang tidak membuat data ganda).
type SubmitResponse struct {
	ID           string    `json:"id"`
	WorkReportID string    `json:"work_report_id"`
	SessionNo    int       `json:"sessionNo"`
	ReceivedAt   time.Time `json:"receivedAt"`
	Duplicate    bool      `json:"duplicate"`
}

func toResponse(wr WorkReport, duplicate bool) SubmitResponse {
	return SubmitResponse{ID: wr.ID, WorkReportID: wr.WorkReportID, SessionNo: wr.SessionNo, ReceivedAt: wr.ReceivedAt, Duplicate: duplicate}
}
