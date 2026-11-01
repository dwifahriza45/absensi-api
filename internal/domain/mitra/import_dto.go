package mitra

const maxImportRows = 5000

type ImportRow struct {
	MitraID string `json:"mitra_id"`
	Name    string `json:"name"`
	Row     int    `json:"-"`
	Problem string `json:"-"`
}
type ImportRequest struct {
	Rows []ImportRow `json:"rows" validate:"required,min=1,max=5000" label:"Data Mitra"`
}
type RowResult struct {
	Row     int    `json:"row"`
	MitraID string `json:"mitra_id,omitempty"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}
type ImportResult struct {
	Total   int         `json:"total"`
	Success int         `json:"success"`
	Skip    int         `json:"skip"`
	Error   int         `json:"error"`
	Rows    []RowResult `json:"rows"`
}
