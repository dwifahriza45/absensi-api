package mitra

import "strings"

type Response struct {
	EmployeeCount int64  `json:"employee_count"`
	CanDelete     bool   `json:"can_delete"`
	DeleteReason  string `json:"delete_reason,omitempty"`
	ID            string `json:"id"`
	MitraID       string `json:"mitra_id"`
	Name          string `json:"name"`
}
type WriteRequest struct {
	Name string `json:"name" validate:"required,max=80" label:"Nama Mitra"`
}

func (r *WriteRequest) Normalize() { r.Name = strings.TrimSpace(r.Name) }
