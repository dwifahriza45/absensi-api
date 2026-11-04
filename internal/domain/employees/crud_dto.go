package employees

import (
	"strings"
)

// WriteRequest identifies employees by NIK and uses master business IDs.
type WriteRequest struct {
	NIK  string `json:"nik" validate:"required,nik" label:"NIK"`
	Name string `json:"name" validate:"required,max=80" label:"Nama Lengkap"`

	MitraID string `json:"mitra_id" validate:"required" label:"Mitra"`

	BidangID string `json:"bidang_id" validate:"required" label:"Bidang"`
	IsActive *bool  `json:"isActive"`
}

func (r *WriteRequest) Normalize() {
	r.Name = strings.Join(strings.Fields(r.Name), " ")
	r.NIK = strings.TrimSpace(r.NIK)
	r.MitraID = strings.TrimSpace(r.MitraID)
	r.BidangID = strings.TrimSpace(r.BidangID)
}

// Status changes do not overwrite employee profile fields.
type StatusRequest struct {
	IsActive *bool `json:"isActive" validate:"required" label:"Status"`
}
