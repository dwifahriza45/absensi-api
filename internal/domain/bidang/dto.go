package bidang

import "strings"

type Response struct {
	EmployeeCount int64  `json:"employee_count"`
	CanDelete     bool   `json:"can_delete"`
	DeleteReason  string `json:"delete_reason,omitempty"`
	ID            string `json:"id"`
	BidangID      string `json:"bidang_id"`
	Name          string `json:"name"`
}
type WriteRequest struct {
	Name string `json:"name" validate:"required,max=60" label:"Nama Bidang"`
}

func (r *WriteRequest) Normalize() { r.Name = strings.TrimSpace(r.Name) }
