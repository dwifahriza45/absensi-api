-- Daerah tidak lagi digunakan; hapus kolom dan sesuaikan index filter aktif.
BEGIN;
DROP INDEX idx_employees_filters;
ALTER TABLE employees DROP COLUMN daerah;
CREATE INDEX idx_employees_filters ON employees(instansi_id, bidang_id) WHERE is_active;
COMMIT;
