-- 000004: Lepas trigger sebelum kolom ID bisnis, lalu hapus generator/counter.
-- UUID dan relasi data tetap dipertahankan.

-- work_reports.
DROP TRIGGER work_reports_public_id ON work_reports;
ALTER TABLE work_reports DROP COLUMN work_report_id;

-- photos.
DROP TRIGGER photos_public_id ON photos;
ALTER TABLE photos DROP COLUMN photo_id;

-- attendances.
DROP TRIGGER attendances_public_id ON attendances;
ALTER TABLE attendances DROP COLUMN attendance_id;

-- instansi.
DROP TRIGGER instansi_public_id ON instansi;
ALTER TABLE instansi DROP COLUMN instansi_id;

-- employees.
DROP TRIGGER employees_public_id ON employees;
ALTER TABLE employees DROP COLUMN user_id;

-- users.
DROP TRIGGER users_public_id ON users;
ALTER TABLE users DROP COLUMN user_id;

-- Generator tidak lagi digunakan setelah seluruh trigger dilepas.
DROP FUNCTION assign_public_id();
DROP FUNCTION next_public_id(TEXT, DATE);
DROP TABLE public_id_counters;
