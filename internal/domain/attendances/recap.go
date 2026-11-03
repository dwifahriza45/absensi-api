package attendances

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/haidarrasyad/absensi-api/internal/domain/photos"
	"github.com/haidarrasyad/absensi-api/internal/pkg/apperror"
	"github.com/haidarrasyad/absensi-api/internal/pkg/httpx"
	"github.com/haidarrasyad/absensi-api/internal/pkg/pdfreport"
)

const MonthRangeMessage = "Pengambilan data dibatasi maksimal 1 bulan. Silakan sesuaikan rentang tanggal pada filter."

// ValidateMonthRange caps the inclusive date range at 31 days.
func ValidateMonthRange(from, to string) error {
	a, e1 := time.Parse(time.DateOnly, from)
	b, e2 := time.Parse(time.DateOnly, to)
	if e1 != nil || e2 != nil || b.Before(a) {
		return ErrInvalidDateRange
	}
	if b.Sub(a).Hours()/24+1 > 31 {
		return apperror.BadRequest("range_too_long", MonthRangeMessage)
	}
	return nil
}

type RecapEmployees func(context.Context, AdminFilter) ([]AdminEmployeeDTO, error)

type Recap struct {
	service   *Service
	employees RecapEmployees
	photos    *photos.Service
	gate      chan struct{}
}

func NewRecap(s *Service, e RecapEmployees, p *photos.Service) *Recap {
	return &Recap{s, e, p, make(chan struct{}, 1)}
}

type recapRow struct {
	employee AdminEmployeeDTO
	date     string
	record   *AdminItem
	status   string
}

func recapStatus(a *AdminItem, today string) string {
	if a == nil {
		return "belum"
	}
	if a.NeedsReview || a.CheckInLocation == nil || a.CheckInLocation.AccuracyM > 100 {
		return "tinjau"
	}
	if a.CheckOutAt != nil {
		return "pulang"
	}
	if a.WorkDate == today {
		return "bekerja"
	}
	return "tanpaPulang"
}

var recapLabels = map[string]string{"belum": "Belum absen", "tinjau": "Perlu ditinjau", "pulang": "Sudah pulang", "bekerja": "Sedang bekerja", "tanpaPulang": "Tanpa absen pulang"}

func (s *Recap) rows(ctx context.Context, r *http.Request) ([]recapRow, error) {
	q := r.URL.Query()
	from, to := q.Get("from"), q.Get("to")
	if err := ValidateMonthRange(from, to); err != nil {
		return nil, err
	}
	if strings.TrimSpace(q.Get("mitra")) == "" {
		return nil, apperror.BadRequest("export_filters_required", "Silakan pilih Mitra terlebih dahulu sebelum mengunduh rekap.")
	}
	status := q.Get("status")
	if status != "" && status != "hadir" && recapLabels[status] == "" {
		return nil, apperror.BadRequest("invalid_status", "Status absensi tidak valid.")
	}
	items, err := s.service.AdminList(ctx, AdminFilter{From: from, To: to, Mitra: q.Get("mitra"), Query: q.Get("q")})
	if err != nil {
		return nil, err
	}
	staff, err := s.employees(ctx, AdminFilter{Mitra: q.Get("mitra"), Query: q.Get("q")})
	if err != nil {
		return nil, err
	}
	employeesByID := map[string]AdminEmployeeDTO{}
	for _, e := range staff {
		employeesByID[e.EmployeeID] = e
	}
	records := map[string]*AdminItem{}
	for i := range items {
		a := &items[i]
		records[a.WorkDate+"|"+a.Employee.EmployeeID] = a
		employeesByID[a.Employee.EmployeeID] = a.Employee
	}
	today := s.service.clock.Now().In(time.FixedZone("WIB", 7*3600)).Format(time.DateOnly)
	all := q.Get("include_absent") == "true" || from == to || q.Get("q") != "" || status == "belum"
	result := []recapRow{}
	start, _ := time.Parse(time.DateOnly, from)
	end, _ := time.Parse(time.DateOnly, to)
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		date := day.Format(time.DateOnly)
		if date > today {
			continue
		}
		for id, e := range employeesByID {
			a := records[date+"|"+id]
			st := recapStatus(a, today)
			if a == nil && !all {
				continue
			}
			if status == "hadir" && a == nil {
				continue
			}
			if status != "" && status != "hadir" && st != status {
				continue
			}
			result = append(result, recapRow{employee: e, date: date, record: a, status: st})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].date != result[j].date {
			return result[i].date > result[j].date
		}
		if result[i].employee.Mitra != result[j].employee.Mitra {
			return result[i].employee.Mitra < result[j].employee.Mitra
		}
		return result[i].employee.Name < result[j].employee.Name
	})
	return result, nil
}
func (s *Recap) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	select {
	case s.gate <- struct{}{}:
		defer func() { <-s.gate }()
	default:
		httpx.Fail(w, r, &apperror.Error{Status: 429, Code: "report_busy", Message: "Rekap sedang diproses. Silakan coba kembali setelah proses sebelumnya selesai."})
		return
	}
	rows, err := s.rows(r.Context(), r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if len(rows) == 0 {
		httpx.Fail(w, r, apperror.BadRequest("empty_report", "Tidak ada data untuk filter yang dipilih."))
		return
	}
	// Export alone may stream for longer; normal API deadlines remain unchanged.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(time.Hour))
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="rekap-absensi-%s-sd-%s.pdf"`, r.URL.Query().Get("from"), r.URL.Query().Get("to")))
	w.Header().Set("Cache-Control", "no-store")
	pdf := pdfreport.New(w)
	for i, row := range rows {
		if err = r.Context().Err(); err != nil {
			break
		}
		err = s.page(r.Context(), pdf, row, i+1, len(rows))
		if err != nil {
			break
		}
	}
	if err == nil {
		err = pdf.Close()
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "attendance PDF stream failed", "error", err)
		return
	}
}
func wrap(s string, n int) []string {
	words := strings.Fields(s)
	lines := []string{}
	line := ""
	for _, word := range words {
		if utf8.RuneCountInString(line+" "+word) > n && line != "" {
			lines = append(lines, line)
			line = ""
		}
		if line != "" {
			line += " "
		}
		line += word
	}
	if line != "" {
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return []string{"-"}
	}
	return lines
}
func (s *Recap) page(ctx context.Context, pdf *pdfreport.Writer, row recapRow, index, total int) error {
	var p pdfreport.Page
	p.Color(.094, .2, .42)
	p.Rect(0, 0, 595, 75)
	p.Color(1, 1, 1)
	p.Text(105, 30, 13, true, "PT Haidar Rasyad Indonesia")
	p.Text(105, 50, 10, false, "REKAP ABSENSI KARYAWAN")
	p.Image(pdfreport.Picture{ID: pdf.Logo, Width: 600, Height: 340}, 36, 13, 54, 45)
	p.Color(.094, .2, .42)
	y := 100.0
	for _, line := range wrap(row.employee.Name, 48) {
		p.Text(36, y, 17, true, line)
		y += 20
	}
	p.Color(.35, .4, .48)
	p.Text(36, y+4, 10, false, row.date+" | "+recapLabels[row.status])
	y += 26
	for _, field := range []struct{ label, value string }{{"Mitra", row.employee.Mitra}, {"Bidang", row.employee.Bidang}} {
		for j, line := range wrap(field.value, 75) {
			if j == 0 {
				p.Text(36, y, 10, true, field.label)
			}
			p.Text(110, y, 10, false, line)
			y += 14
		}
		y += 6
	}
	record := row.record
	clock := func(t *time.Time) string {
		if t == nil {
			return "Belum tercatat"
		}
		return t.In(time.FixedZone("WIB", 7*3600)).Format("15.04") + " WIB"
	}
	location := func(l *LocationDTO) string {
		if l == nil {
			return "Belum tercatat"
		}
		return fmt.Sprintf("%.5f, %.5f (+/- %.0f m)", l.Lat, l.Lng, l.AccuracyM)
	}
	p.Color(.094, .2, .42)
	p.Text(36, y+4, 11, true, "RINGKASAN KEHADIRAN")
	y += 24
	in, out, locIn, locOut := "Belum absen", "Belum absen pulang", "Belum tercatat", "Belum tercatat"
	if record != nil {
		in = clock(&record.CheckInAt)
		out = clock(record.CheckOutAt)
		locIn = location(record.CheckInLocation)
		locOut = location(record.CheckOutLocation)
	}
	p.Color(.15, .18, .23)
	for _, f := range []struct{ a, b string }{{"Masuk", in}, {"Lokasi masuk", locIn}, {"Pulang", out}, {"Lokasi pulang", locOut}} {
		p.Text(36, y, 9, false, f.a)
		p.Text(140, y, 10, true, f.b)
		y += 18
	}
	if record != nil {
		notes := record.CheckOutReason
		if notes != nil {
			for j, line := range wrap(*notes, 78) {
				if j == 0 {
					p.Text(36, y, 9, false, "Alasan pulang")
				}
				p.Text(140, y, 9, false, line)
				y += 13
			}
		}
		if record.EarlyLeave != nil {
			for j, line := range wrap(clock(&record.EarlyLeave.RequestedAt)+" - "+record.EarlyLeave.Reason, 78) {
				if j == 0 {
					p.Text(36, y, 9, false, "Pulang awal")
				}
				p.Text(140, y, 9, false, line)
				y += 13
			}
		}
	}
	y += 10
	draw := func(id *string, x, top, width, height float64, label string) error {
		p.Color(.95, .96, .98)
		p.Rect(x, top, width, height)
		p.Color(.35, .4, .48)
		if id == nil {
			p.Text(x+8, top+height/2, 9, false, "Belum ada foto")
		} else {
			_, reader, err := s.photos.Open(ctx, *id)
			if err != nil {
				return err
			}
			picture, err := pdf.Image(reader)
			_ = reader.Close()
			if err != nil {
				return err
			}
			p.Image(picture, x, top, width, height)
		}
		p.Color(.094, .2, .42)
		p.Text(x, top+height+14, 9, true, label)
		return nil
	}
	var selfieIn, selfieOut *string
	if record != nil {
		selfieIn = record.CheckInSelfieID
		selfieOut = record.CheckOutSelfieID
	}
	if err := draw(selfieIn, 36, y, 250, 125, "SELFIE MASUK"); err != nil {
		return err
	}
	if err := draw(selfieOut, 309, y, 250, 125, "SELFIE PULANG"); err != nil {
		return err
	}
	y += 166
	p.Text(36, y, 11, true, "DOKUMENTASI PEKERJAAN")
	y += 14
	for no := 1; no <= 3; no++ {
		var id *string
		label := fmt.Sprintf("SESI %d - Belum dikirim", no)
		if record != nil {
			for _, session := range record.Sessions {
				if session.No == no {
					copyID := session.PhotoID
					id = &copyID
					label = fmt.Sprintf("SESI %d - %s", no, clock(&session.ReceivedAt))
				}
			}
		}
		if err := draw(id, 36+float64(no-1)*177, y, 169, 94, label); err != nil {
			return err
		}
	}
	p.Color(.4, .45, .52)
	p.Text(36, 813, 8, false, "Dokumen internal - berisi data pribadi karyawan")
	p.Text(510, 813, 8, false, fmt.Sprintf("%d / %d", index, total))
	return pdf.Page(&p)
}
