package employees

import (
	"strings"
)

// WriteRequest uses an existing institution business ID. Phone is optional;
// leaving it blank on update preserves the existing contact phone.
type WriteRequest struct {
	NIK     string `json:"nik" validate:"required,nik" label:"NIK"`
	Name    string `json:"name" validate:"required,max=80" label:"Nama Lengkap"`
	Phone   string `json:"phone" validate:"omitempty,phone,max=32" label:"Nomor HP"`
	MitraID string `json:"mitra_id" validate:"required" label:"Mitra"`

	BidangID string `json:"bidang_id" validate:"required" label:"Bidang"`
	IsActive *bool  `json:"isActive"`
}

func (r *WriteRequest) Normalize() {
	r.Name = strings.Join(strings.Fields(r.Name), " ")
	r.NIK = strings.TrimSpace(r.NIK)
	r.Phone = strings.TrimSpace(r.Phone)
	r.MitraID = strings.TrimSpace(r.MitraID)
	r.BidangID = strings.TrimSpace(r.BidangID)
}

// Status changes do not overwrite employee profile fields.
type StatusRequest struct {
	IsActive *bool `json:"isActive" validate:"required" label:"Status"`
}
