-- 000003: Kembalikan nama instansi ke kolom teks tanpa kehilangan nama master.

-- 1. Pulihkan nama sebelum menghapus relasinya.
ALTER TABLE employees
    ADD COLUMN instansi TEXT NOT NULL DEFAULT '';

UPDATE employees e
SET instansi = i.name
FROM instansi i
WHERE i.id = e.instansi_id;

-- 2. Lepas index, foreign key, dan tabel master.
DROP INDEX idx_employees_filters;
DROP INDEX idx_employees_instansi_id;
ALTER TABLE employees DROP COLUMN instansi_id;
DROP TABLE instansi;

-- 3. Pulihkan index dengan struktur awal.
CREATE INDEX idx_employees_filters
    ON employees (instansi, daerah, bidang)
    WHERE is_active;
