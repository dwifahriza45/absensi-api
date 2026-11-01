-- Rollback memulihkan struktur saja. Nilai daerah yang dihapus tidak dipulihkan.
BEGIN;
DROP INDEX idx_employees_filters;
ALTER TABLE employees ADD COLUMN daerah TEXT NOT NULL DEFAULT '';
CREATE INDEX idx_employees_filters ON employees(instansi_id, daerah, bidang_id) WHERE is_active;
COMMIT;
