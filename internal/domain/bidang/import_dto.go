package bidang

const maxImportRows = 5000

type ImportRow struct {
	BidangID string `json:"bidang_id"`
	Name     string `json:"name"`
	Row      int    `json:"-"`
	Problem  string `json:"-"`
}
type ImportRequest struct {
	Rows []ImportRow `json:"rows" validate:"required,min=1,max=5000" label:"Data Bidang"`
}
type RowResult struct {
	Row      int    `json:"row"`
	BidangID string `json:"bidang_id,omitempty"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Message  string `json:"message"`
}
type ImportResult struct {
	Total   int         `json:"total"`
	Success int         `json:"success"`
	Skip    int         `json:"skip"`
	Error   int         `json:"error"`
	Rows    []RowResult `json:"rows"`
}
