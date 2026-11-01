package bidang

import (
	"context"
	"fmt"
	"github.com/haidarrasyad/absensi-api/internal/pkg/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

const columns = `id::text,bidang_id,name,(SELECT count(*) FROM employees e WHERE e.bidang_id=bidang.bidang_id)`

func scan(row pgx.Row) (Response, error) {
	var item Response
	err := row.Scan(&item.ID, &item.BidangID, &item.Name, &item.EmployeeCount)
	item.CanDelete = item.EmployeeCount == 0
	if !item.CanDelete {
		item.DeleteReason = fmt.Sprintf("Bidang masih digunakan oleh %d karyawan.", item.EmployeeCount)
	}
	return item, err
}
func (r *Repository) List(ctx context.Context, q string) ([]Response, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	rows, err := r.db.Query(ctx, `SELECT `+columns+` FROM bidang WHERE $1='' OR strpos(lower(name),lower($1))>0 ORDER BY name,bidang_id`, q)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Response, error) { return scan(row) })
}
func (r *Repository) Get(ctx context.Context, id string) (Response, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return scan(r.db.QueryRow(ctx, `SELECT `+columns+` FROM bidang WHERE bidang_id=$1`, id))
}
func (r *Repository) Save(ctx context.Context, id, name string) (item Response, err error) {
	// Share employee import/edit lock; FK additionally protects concurrent deletion.
	err = database.NewCoordinator(r.db).Within(ctx, "employees:import", func(ctx context.Context) error {
		q := database.Executor(ctx, r.db)
		var inner error
		if id == "" {
			item, inner = scan(q.QueryRow(ctx, `INSERT INTO bidang(name) VALUES($1) RETURNING `+columns, name))
		} else {
			item, inner = scan(q.QueryRow(ctx, `UPDATE bidang SET name=$2,updated_at=now() WHERE bidang_id=$1 RETURNING `+columns, id, name))
		}
		return inner
	})
	return item, err
}
func (r *Repository) Delete(ctx context.Context, id string) error {
	return database.NewCoordinator(r.db).Within(ctx, "employees:import", func(ctx context.Context) error {
		tag, err := database.Executor(ctx, r.db).Exec(ctx, `DELETE FROM bidang WHERE bidang_id=$1`, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return nil
	})
}
