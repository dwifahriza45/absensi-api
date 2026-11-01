-- 000006: Relasi memakai ID bisnis; UUID id tetap menjadi primary key.
BEGIN;

-- 1. Pisahkan ID karyawan (EMP) dari ID akun admin (USER).
DROP TRIGGER employees_public_id ON employees;
ALTER TABLE employees DROP CONSTRAINT employees_user_id_format;
ALTER TABLE employees RENAME COLUMN user_id TO employee_id;
ALTER TABLE employees RENAME CONSTRAINT employees_user_id_key TO employees_employee_id_key;

UPDATE employees SET employee_id = 'EMP-' || substr(employee_id, 6);

ALTER TABLE employees ADD CONSTRAINT employees_employee_id_format
    CHECK (employee_id ~ '^EMP-[0-9]{8}-[0-9]{6}$');

INSERT INTO public_id_counters (prefix, id_date, last_value)
SELECT 'EMP', to_date(substr(employee_id, 5, 8), 'YYYYMMDD'),
       max(split_part(employee_id, '-', 3)::integer)
FROM employees
GROUP BY to_date(substr(employee_id, 5, 8), 'YYYYMMDD')
ON CONFLICT (prefix, id_date) DO UPDATE
    SET last_value = greatest(public_id_counters.last_value, EXCLUDED.last_value);

CREATE TRIGGER employees_public_id
    BEFORE INSERT ON employees
    FOR EACH ROW EXECUTE FUNCTION assign_public_id('employee_id', 'EMP', 'created_at', 'phone_hash');

-- 2. Lepas FK UUID sebelum mengubah isi dan tipe kolom referensi.
ALTER TABLE employees DROP CONSTRAINT employees_instansi_id_fkey;
ALTER TABLE attendances DROP CONSTRAINT attendances_employee_id_fkey;
ALTER TABLE attendances DROP CONSTRAINT attendances_check_in_selfie_id_fkey;
ALTER TABLE attendances DROP CONSTRAINT attendances_check_out_selfie_id_fkey;
ALTER TABLE work_reports DROP CONSTRAINT work_reports_attendance_id_fkey;
ALTER TABLE work_reports DROP CONSTRAINT work_reports_photo_id_fkey;

ALTER TABLE employees ALTER COLUMN instansi_id TYPE TEXT USING instansi_id::text;
UPDATE employees child
SET instansi_id = parent.instansi_id
FROM instansi parent
WHERE child.instansi_id = parent.id::text;

ALTER TABLE attendances ALTER COLUMN employee_id TYPE TEXT USING employee_id::text;
UPDATE attendances child
SET employee_id = parent.employee_id
FROM employees parent
WHERE child.employee_id = parent.id::text;

ALTER TABLE attendances ALTER COLUMN check_in_selfie_id TYPE TEXT USING check_in_selfie_id::text;
UPDATE attendances child
SET check_in_selfie_id = parent.photo_id
FROM photos parent
WHERE child.check_in_selfie_id = parent.id::text;

ALTER TABLE attendances ALTER COLUMN check_out_selfie_id TYPE TEXT USING check_out_selfie_id::text;
UPDATE attendances child
SET check_out_selfie_id = parent.photo_id
FROM photos parent
WHERE child.check_out_selfie_id = parent.id::text;

ALTER TABLE work_reports ALTER COLUMN attendance_id TYPE TEXT USING attendance_id::text;
UPDATE work_reports child
SET attendance_id = parent.attendance_id
FROM attendances parent
WHERE child.attendance_id = parent.id::text;

ALTER TABLE work_reports ALTER COLUMN photo_id TYPE TEXT USING photo_id::text;
UPDATE work_reports child
SET photo_id = parent.photo_id
FROM photos parent
WHERE child.photo_id = parent.id::text;

-- 3. Pasang FK ke kolom bisnis yang UNIQUE.
ALTER TABLE employees
    ADD CONSTRAINT employees_instansi_id_fkey FOREIGN KEY (instansi_id)
    REFERENCES instansi (instansi_id) ON UPDATE RESTRICT ON DELETE RESTRICT;
ALTER TABLE attendances
    ADD CONSTRAINT attendances_employee_id_fkey FOREIGN KEY (employee_id)
    REFERENCES employees (employee_id) ON UPDATE RESTRICT ON DELETE NO ACTION;
ALTER TABLE attendances
    ADD CONSTRAINT attendances_check_in_selfie_id_fkey FOREIGN KEY (check_in_selfie_id)
    REFERENCES photos (photo_id) ON UPDATE RESTRICT ON DELETE NO ACTION;
ALTER TABLE attendances
    ADD CONSTRAINT attendances_check_out_selfie_id_fkey FOREIGN KEY (check_out_selfie_id)
    REFERENCES photos (photo_id) ON UPDATE RESTRICT ON DELETE NO ACTION;
ALTER TABLE work_reports
    ADD CONSTRAINT work_reports_attendance_id_fkey FOREIGN KEY (attendance_id)
    REFERENCES attendances (attendance_id) ON UPDATE RESTRICT ON DELETE CASCADE;
ALTER TABLE work_reports
    ADD CONSTRAINT work_reports_photo_id_fkey FOREIGN KEY (photo_id)
    REFERENCES photos (photo_id) ON UPDATE RESTRICT ON DELETE NO ACTION;

-- 4. Sesi juga mempunyai ID bisnis; JWT memakai session_id ini.
ALTER TABLE sessions ADD COLUMN session_id TEXT;
DO $$
DECLARE
    row_record RECORD;
BEGIN
    FOR row_record IN
        SELECT id, (created_at AT TIME ZONE 'Asia/Jakarta')::date AS id_date
        FROM sessions ORDER BY created_at, id
    LOOP
        UPDATE sessions SET session_id = next_public_id('SES', row_record.id_date)
        WHERE id = row_record.id;
    END LOOP;
END;
$$;
ALTER TABLE sessions ALTER COLUMN session_id SET NOT NULL;
ALTER TABLE sessions ADD CONSTRAINT sessions_session_id_key UNIQUE (session_id);
ALTER TABLE sessions ADD CONSTRAINT sessions_session_id_format
    CHECK (session_id ~ '^SES-[0-9]{8}-[0-9]{6}$');
CREATE TRIGGER sessions_public_id
    BEFORE INSERT ON sessions
    FOR EACH ROW EXECUTE FUNCTION assign_public_id('session_id', 'SES', 'created_at', 'refresh_hash');

-- 5. Pemilik sesi memakai FK eksplisit ke USER atau EMP sesuai role.
ALTER TABLE sessions ADD COLUMN user_id TEXT
    REFERENCES users (user_id) ON UPDATE RESTRICT ON DELETE CASCADE;
ALTER TABLE sessions ADD COLUMN employee_id TEXT
    REFERENCES employees (employee_id) ON UPDATE RESTRICT ON DELETE CASCADE;

UPDATE sessions s SET user_id = u.user_id FROM users u
WHERE s.role = 'admin' AND s.subject = u.id;
UPDATE sessions s SET employee_id = e.employee_id FROM employees e
WHERE s.role = 'karyawan' AND s.subject = e.id;

-- Cabut sesi yang pemiliknya sudah dihapus.
DELETE FROM sessions
WHERE (role = 'admin' AND user_id IS NULL)
   OR (role = 'karyawan' AND employee_id IS NULL);
ALTER TABLE sessions ADD CONSTRAINT sessions_owner_role CHECK (
    (role = 'admin' AND user_id IS NOT NULL AND employee_id IS NULL)
    OR (role = 'karyawan' AND employee_id IS NOT NULL AND user_id IS NULL)
);
ALTER TABLE sessions DROP COLUMN subject;
CREATE INDEX idx_sessions_user_id ON sessions (user_id);
CREATE INDEX idx_sessions_employee_id ON sessions (employee_id);
CREATE INDEX idx_attendances_check_in_selfie_id ON attendances (check_in_selfie_id);
CREATE INDEX idx_attendances_check_out_selfie_id ON attendances (check_out_selfie_id);
CREATE INDEX idx_work_reports_photo_id ON work_reports (photo_id);
COMMIT;
