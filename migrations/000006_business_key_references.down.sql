-- 000006: Kembalikan referensi UUID sebelum mengembalikan kode karyawan.
BEGIN;
DROP INDEX idx_sessions_user_id;
DROP INDEX idx_sessions_employee_id;
DROP INDEX idx_attendances_check_in_selfie_id;
DROP INDEX idx_attendances_check_out_selfie_id;
DROP INDEX idx_work_reports_photo_id;

ALTER TABLE sessions ADD COLUMN subject UUID;
UPDATE sessions s SET subject = u.id FROM users u
WHERE s.role = 'admin' AND s.user_id = u.user_id;
UPDATE sessions s SET subject = e.id FROM employees e
WHERE s.role = 'karyawan' AND s.employee_id = e.employee_id;
DELETE FROM sessions WHERE subject IS NULL;
ALTER TABLE sessions ALTER COLUMN subject SET NOT NULL;
ALTER TABLE sessions DROP CONSTRAINT sessions_owner_role;
ALTER TABLE sessions DROP COLUMN user_id;
ALTER TABLE sessions DROP COLUMN employee_id;
DROP TRIGGER sessions_public_id ON sessions;
ALTER TABLE sessions DROP COLUMN session_id;
DELETE FROM public_id_counters WHERE prefix = 'SES';

ALTER TABLE employees DROP CONSTRAINT employees_instansi_id_fkey;
ALTER TABLE attendances DROP CONSTRAINT attendances_employee_id_fkey;
ALTER TABLE attendances DROP CONSTRAINT attendances_check_in_selfie_id_fkey;
ALTER TABLE attendances DROP CONSTRAINT attendances_check_out_selfie_id_fkey;
ALTER TABLE work_reports DROP CONSTRAINT work_reports_attendance_id_fkey;
ALTER TABLE work_reports DROP CONSTRAINT work_reports_photo_id_fkey;

UPDATE work_reports child
SET photo_id = parent.id::text
FROM photos parent
WHERE child.photo_id = parent.photo_id;
ALTER TABLE work_reports ALTER COLUMN photo_id TYPE UUID USING photo_id::uuid;

UPDATE work_reports child
SET attendance_id = parent.id::text
FROM attendances parent
WHERE child.attendance_id = parent.attendance_id;
ALTER TABLE work_reports ALTER COLUMN attendance_id TYPE UUID USING attendance_id::uuid;

UPDATE attendances child
SET check_out_selfie_id = parent.id::text
FROM photos parent
WHERE child.check_out_selfie_id = parent.photo_id;
ALTER TABLE attendances ALTER COLUMN check_out_selfie_id TYPE UUID USING check_out_selfie_id::uuid;

UPDATE attendances child
SET check_in_selfie_id = parent.id::text
FROM photos parent
WHERE child.check_in_selfie_id = parent.photo_id;
ALTER TABLE attendances ALTER COLUMN check_in_selfie_id TYPE UUID USING check_in_selfie_id::uuid;

UPDATE attendances child
SET employee_id = parent.id::text
FROM employees parent
WHERE child.employee_id = parent.employee_id;
ALTER TABLE attendances ALTER COLUMN employee_id TYPE UUID USING employee_id::uuid;

UPDATE employees child
SET instansi_id = parent.id::text
FROM instansi parent
WHERE child.instansi_id = parent.instansi_id;
ALTER TABLE employees ALTER COLUMN instansi_id TYPE UUID USING instansi_id::uuid;

ALTER TABLE employees
    ADD CONSTRAINT employees_instansi_id_fkey FOREIGN KEY (instansi_id)
    REFERENCES instansi (id) ON DELETE RESTRICT;
ALTER TABLE attendances
    ADD CONSTRAINT attendances_employee_id_fkey FOREIGN KEY (employee_id)
    REFERENCES employees (id) ON DELETE NO ACTION;
ALTER TABLE attendances
    ADD CONSTRAINT attendances_check_in_selfie_id_fkey FOREIGN KEY (check_in_selfie_id)
    REFERENCES photos (id) ON DELETE NO ACTION;
ALTER TABLE attendances
    ADD CONSTRAINT attendances_check_out_selfie_id_fkey FOREIGN KEY (check_out_selfie_id)
    REFERENCES photos (id) ON DELETE NO ACTION;
ALTER TABLE work_reports
    ADD CONSTRAINT work_reports_attendance_id_fkey FOREIGN KEY (attendance_id)
    REFERENCES attendances (id) ON DELETE CASCADE;
ALTER TABLE work_reports
    ADD CONSTRAINT work_reports_photo_id_fkey FOREIGN KEY (photo_id)
    REFERENCES photos (id) ON DELETE NO ACTION;

DROP TRIGGER employees_public_id ON employees;
ALTER TABLE employees DROP CONSTRAINT employees_employee_id_format;
ALTER TABLE employees RENAME COLUMN employee_id TO user_id;
ALTER TABLE employees RENAME CONSTRAINT employees_employee_id_key TO employees_user_id_key;
-- Alokasikan USER baru agar tidak bertabrakan dengan admin yang dibuat setelah upgrade.
DO $$
DECLARE
    row_record RECORD;
BEGIN
    FOR row_record IN
        SELECT id, (created_at AT TIME ZONE 'Asia/Jakarta')::date AS id_date
        FROM employees ORDER BY created_at, id
    LOOP
        UPDATE employees SET user_id = next_public_id('USER', row_record.id_date)
        WHERE id = row_record.id;
    END LOOP;
END;
$$;
ALTER TABLE employees ADD CONSTRAINT employees_user_id_format
    CHECK (user_id ~ '^USER-[0-9]{8}-[0-9]{6}$');
CREATE TRIGGER employees_public_id
    BEFORE INSERT ON employees
    FOR EACH ROW EXECUTE FUNCTION assign_public_id('user_id', 'USER', 'created_at', 'phone_hash');
DELETE FROM public_id_counters WHERE prefix = 'EMP';
COMMIT;
