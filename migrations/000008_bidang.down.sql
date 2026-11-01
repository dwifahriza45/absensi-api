-- Kembalikan nama bidang tanpa menghilangkan label pada employee.
BEGIN;
ALTER TABLE employees ADD COLUMN bidang TEXT NOT NULL DEFAULT '';
UPDATE employees e SET bidang = b.name FROM bidang b WHERE b.bidang_id = e.bidang_id;
DROP INDEX idx_employees_filters;
ALTER TABLE employees DROP COLUMN bidang_id;
CREATE INDEX idx_employees_filters ON employees(instansi_id, daerah, bidang) WHERE is_active;
DROP TABLE bidang;
-- Counter BID dipertahankan agar nomor yang pernah dialokasikan tidak dipakai ulang.
COMMIT;
