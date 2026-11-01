-- 000004: Tambahkan ID bisnis PREFIX-YYYYMMDD-000001.
-- UUID primary key dan foreign key yang sudah ada tetap dipertahankan.
-- Users dan employees berbagi counter USER agar kode tidak bertabrakan.

-- Counter terpisah per prefix/tanggal; increment atomik saat request bersamaan.
CREATE TABLE public_id_counters (
    prefix      TEXT    NOT NULL,
    id_date     DATE    NOT NULL,
    last_value  INTEGER NOT NULL CHECK (last_value BETWEEN 1 AND 999999),

    PRIMARY KEY (prefix, id_date)
);

-- Alokasikan nomor berikutnya dalam transaksi pemanggil.
CREATE FUNCTION next_public_id(p_prefix TEXT, p_date DATE)
RETURNS TEXT
LANGUAGE plpgsql AS $$
DECLARE
    n INTEGER;
BEGIN
    INSERT INTO public_id_counters (prefix, id_date, last_value)
    VALUES (p_prefix, p_date, 1)
    ON CONFLICT (prefix, id_date) DO UPDATE
        SET last_value = public_id_counters.last_value + 1
    RETURNING last_value INTO n;

    RETURN p_prefix || '-' || to_char(p_date, 'YYYYMMDD') || '-' || lpad(n::text, 6, '0');
END;
$$;

-- Argumen trigger:
--   0: kolom ID bisnis; 1: prefix; 2: kolom waktu pembuatan;
--   3 dan seterusnya: kolom natural key untuk mengenali upsert/import ulang.
CREATE FUNCTION assign_public_id()
RETURNS TRIGGER
LANGUAGE plpgsql AS $$
DECLARE
    code TEXT;
    predicates TEXT := '';
    arg INTEGER;
BEGIN
    -- Cari ID lama agar import ulang tidak mengalokasikan nomor baru.
    FOR arg IN 3..TG_NARGS-1 LOOP
        predicates := predicates
            || CASE WHEN predicates = '' THEN '' ELSE ' AND ' END
            || format('%I::text = $1->>%L', TG_ARGV[arg], TG_ARGV[arg]);
    END LOOP;

    EXECUTE format(
        'SELECT %I FROM %I.%I WHERE %s',
        TG_ARGV[0], TG_TABLE_SCHEMA, TG_TABLE_NAME, predicates
    )
    INTO code USING to_jsonb(NEW);

    IF code IS NULL THEN
        code := next_public_id(
            TG_ARGV[1],
            ((to_jsonb(NEW)->>TG_ARGV[2])::timestamptz AT TIME ZONE 'Asia/Jakarta')::date
        );
    END IF;

    NEW := jsonb_populate_record(NEW, jsonb_build_object(TG_ARGV[0], code));
    RETURN NEW;
END;
$$;

-- Admin: user_id = USER-YYYYMMDD-000001.
ALTER TABLE users ADD COLUMN user_id TEXT;

-- Backfill berurutan menurut waktu pembuatan, lalu UUID untuk waktu yang sama.
DO $$
DECLARE
    row_record RECORD;
BEGIN
    FOR row_record IN
        SELECT id, (created_at AT TIME ZONE 'Asia/Jakarta')::date AS id_date
        FROM users
        ORDER BY created_at, id
    LOOP
        UPDATE users
        SET user_id = next_public_id('USER', row_record.id_date)
        WHERE id = row_record.id;
    END LOOP;
END;
$$;

ALTER TABLE users ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE users
    ADD CONSTRAINT users_user_id_key UNIQUE (user_id);
ALTER TABLE users
    ADD CONSTRAINT users_user_id_format
    CHECK (user_id ~ '^USER-[0-9]{8}-[0-9]{6}$');

CREATE TRIGGER users_public_id
    BEFORE INSERT ON users
    FOR EACH ROW EXECUTE FUNCTION assign_public_id('user_id', 'USER', 'created_at', 'email');

-- Karyawan: user_id = USER-YYYYMMDD-000001.
ALTER TABLE employees ADD COLUMN user_id TEXT;

-- Backfill berurutan menurut waktu pembuatan, lalu UUID untuk waktu yang sama.
DO $$
DECLARE
    row_record RECORD;
BEGIN
    FOR row_record IN
        SELECT id, (created_at AT TIME ZONE 'Asia/Jakarta')::date AS id_date
        FROM employees
        ORDER BY created_at, id
    LOOP
        UPDATE employees
        SET user_id = next_public_id('USER', row_record.id_date)
        WHERE id = row_record.id;
    END LOOP;
END;
$$;

ALTER TABLE employees ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE employees
    ADD CONSTRAINT employees_user_id_key UNIQUE (user_id);
ALTER TABLE employees
    ADD CONSTRAINT employees_user_id_format
    CHECK (user_id ~ '^USER-[0-9]{8}-[0-9]{6}$');

CREATE TRIGGER employees_public_id
    BEFORE INSERT ON employees
    FOR EACH ROW EXECUTE FUNCTION assign_public_id('user_id', 'USER', 'created_at', 'phone_hash');

-- Instansi: instansi_id = INT-YYYYMMDD-000001.
ALTER TABLE instansi ADD COLUMN instansi_id TEXT;

-- Backfill berurutan menurut waktu pembuatan, lalu UUID untuk waktu yang sama.
DO $$
DECLARE
    row_record RECORD;
BEGIN
    FOR row_record IN
        SELECT id, (created_at AT TIME ZONE 'Asia/Jakarta')::date AS id_date
        FROM instansi
        ORDER BY created_at, id
    LOOP
        UPDATE instansi
        SET instansi_id = next_public_id('INT', row_record.id_date)
        WHERE id = row_record.id;
    END LOOP;
END;
$$;

ALTER TABLE instansi ALTER COLUMN instansi_id SET NOT NULL;
ALTER TABLE instansi
    ADD CONSTRAINT instansi_instansi_id_key UNIQUE (instansi_id);
ALTER TABLE instansi
    ADD CONSTRAINT instansi_instansi_id_format
    CHECK (instansi_id ~ '^INT-[0-9]{8}-[0-9]{6}$');

CREATE TRIGGER instansi_public_id
    BEFORE INSERT ON instansi
    FOR EACH ROW EXECUTE FUNCTION assign_public_id('instansi_id', 'INT', 'created_at', 'name');

-- Absensi: attendance_id = ATT-YYYYMMDD-000001.
ALTER TABLE attendances ADD COLUMN attendance_id TEXT;

-- Backfill berurutan menurut waktu pembuatan, lalu UUID untuk waktu yang sama.
DO $$
DECLARE
    row_record RECORD;
BEGIN
    FOR row_record IN
        SELECT id, (created_at AT TIME ZONE 'Asia/Jakarta')::date AS id_date
        FROM attendances
        ORDER BY created_at, id
    LOOP
        UPDATE attendances
        SET attendance_id = next_public_id('ATT', row_record.id_date)
        WHERE id = row_record.id;
    END LOOP;
END;
$$;

ALTER TABLE attendances ALTER COLUMN attendance_id SET NOT NULL;
ALTER TABLE attendances
    ADD CONSTRAINT attendances_attendance_id_key UNIQUE (attendance_id);
ALTER TABLE attendances
    ADD CONSTRAINT attendances_attendance_id_format
    CHECK (attendance_id ~ '^ATT-[0-9]{8}-[0-9]{6}$');

CREATE TRIGGER attendances_public_id
    BEFORE INSERT ON attendances
    FOR EACH ROW EXECUTE FUNCTION assign_public_id('attendance_id', 'ATT', 'created_at', 'employee_id', 'work_date');

-- Foto: photo_id = PHO-YYYYMMDD-000001.
ALTER TABLE photos ADD COLUMN photo_id TEXT;

-- Backfill berurutan menurut waktu pembuatan, lalu UUID untuk waktu yang sama.
DO $$
DECLARE
    row_record RECORD;
BEGIN
    FOR row_record IN
        SELECT id, (created_at AT TIME ZONE 'Asia/Jakarta')::date AS id_date
        FROM photos
        ORDER BY created_at, id
    LOOP
        UPDATE photos
        SET photo_id = next_public_id('PHO', row_record.id_date)
        WHERE id = row_record.id;
    END LOOP;
END;
$$;

ALTER TABLE photos ALTER COLUMN photo_id SET NOT NULL;
ALTER TABLE photos
    ADD CONSTRAINT photos_photo_id_key UNIQUE (photo_id);
ALTER TABLE photos
    ADD CONSTRAINT photos_photo_id_format
    CHECK (photo_id ~ '^PHO-[0-9]{8}-[0-9]{6}$');

CREATE TRIGGER photos_public_id
    BEFORE INSERT ON photos
    FOR EACH ROW EXECUTE FUNCTION assign_public_id('photo_id', 'PHO', 'created_at', 'storage_key');

-- Laporan kerja: work_report_id = WRP-YYYYMMDD-000001.
ALTER TABLE work_reports ADD COLUMN work_report_id TEXT;

-- Backfill berurutan menurut waktu pembuatan, lalu UUID untuk waktu yang sama.
DO $$
DECLARE
    row_record RECORD;
BEGIN
    FOR row_record IN
        SELECT id, (received_at AT TIME ZONE 'Asia/Jakarta')::date AS id_date
        FROM work_reports
        ORDER BY received_at, id
    LOOP
        UPDATE work_reports
        SET work_report_id = next_public_id('WRP', row_record.id_date)
        WHERE id = row_record.id;
    END LOOP;
END;
$$;

ALTER TABLE work_reports ALTER COLUMN work_report_id SET NOT NULL;
ALTER TABLE work_reports
    ADD CONSTRAINT work_reports_work_report_id_key UNIQUE (work_report_id);
ALTER TABLE work_reports
    ADD CONSTRAINT work_reports_work_report_id_format
    CHECK (work_report_id ~ '^WRP-[0-9]{8}-[0-9]{6}$');

CREATE TRIGGER work_reports_public_id
    BEFORE INSERT ON work_reports
    FOR EACH ROW EXECUTE FUNCTION assign_public_id('work_report_id', 'WRP', 'received_at', 'attendance_id', 'session_no');
