-- 000003: Ubah nama instansi pada employees menjadi relasi ke tabel master.

-- 1. Buat master instansi.
CREATE TABLE instansi (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT        NOT NULL UNIQUE CHECK (btrim(name) <> ''),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 2. Salin nama lama. Nama kosong tidak dibuatkan master instansi.
INSERT INTO instansi (name)
SELECT DISTINCT btrim(instansi)
FROM employees
WHERE btrim(instansi) <> '';

-- 3. Hubungkan pegawai ke master. Instansi yang dipakai tidak boleh dihapus.
ALTER TABLE employees
    ADD COLUMN instansi_id UUID REFERENCES instansi (id) ON DELETE RESTRICT;

UPDATE employees e
SET instansi_id = i.id
FROM instansi i
WHERE i.name = btrim(e.instansi);

-- 4. Hapus kolom teks lama dan buat ulang index menggunakan foreign key.
DROP INDEX idx_employees_filters;
ALTER TABLE employees DROP COLUMN instansi;

CREATE INDEX idx_employees_instansi_id ON employees (instansi_id);
CREATE INDEX idx_employees_filters
    ON employees (instansi_id, daerah, bidang)
    WHERE is_active;
