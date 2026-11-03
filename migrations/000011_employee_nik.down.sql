ALTER TABLE employees DROP COLUMN nik;
DELETE FROM sessions WHERE employee_id IS NOT NULL;
