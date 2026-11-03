-- Existing employees remain without NIK until an admin supplies the real value.
ALTER TABLE employees ADD COLUMN nik TEXT;
ALTER TABLE employees ADD CONSTRAINT employees_nik_key UNIQUE (nik);
ALTER TABLE employees ADD CONSTRAINT employees_nik_format CHECK (nik IS NULL OR nik ~ '^[0-9]{16}$');
-- Require re-identification using the new credentials after migration.
DELETE FROM sessions WHERE employee_id IS NOT NULL;
