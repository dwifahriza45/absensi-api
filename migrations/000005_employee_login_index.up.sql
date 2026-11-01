-- 000005: Index untuk pencocokan nama lengkap pegawai aktif saat login.
-- Ekspresi harus sesuai query: lower(trim(name)) / lower(btrim(name)).
CREATE INDEX idx_employees_login_name
    ON employees (lower(btrim(name)))
    WHERE is_active;
