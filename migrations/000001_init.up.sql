-- 000001: Skema awal pegawai, foto, absensi, dan laporan kerja.
-- Instansi awalnya berupa teks; migration 000003 memindahkannya ke tabel master.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Pegawai.
CREATE TABLE employees (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT        NOT NULL,
    instansi    TEXT        NOT NULL DEFAULT '',
    daerah      TEXT        NOT NULL DEFAULT '',
    bidang      TEXT        NOT NULL DEFAULT '',
    phone_hash  TEXT        NOT NULL UNIQUE, -- HMAC nomor HP, nomor asli tidak disimpan
    is_active   BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_employees_name ON employees (lower(name));

CREATE INDEX idx_employees_filters ON employees (instansi, daerah, bidang) WHERE is_active;

-- Metadata foto; file disimpan di storage privat.
CREATE TABLE photos (
    id            UUID PRIMARY KEY,
    kind          TEXT        NOT NULL CHECK (kind IN ('selfie_in', 'selfie_out', 'work_report')),
    storage_key   TEXT        NOT NULL UNIQUE,
    content_type  TEXT        NOT NULL,
    size_bytes    BIGINT      NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Absensi harian: satu record per pegawai/tanggal.
CREATE TABLE attendances (
    id                        UUID PRIMARY KEY,
    employee_id               UUID        NOT NULL REFERENCES employees (id),
    work_date                 DATE        NOT NULL,

    -- Absen masuk.
    check_in_at               TIMESTAMPTZ NOT NULL,
    check_in_lat              DOUBLE PRECISION,
    check_in_lng              DOUBLE PRECISION,
    check_in_accuracy_m       DOUBLE PRECISION,
    check_in_selfie_id        UUID REFERENCES photos (id),
    check_in_flag             BOOLEAN     NOT NULL DEFAULT FALSE, -- perlu ditinjau PIC

    -- Absen pulang.
    check_out_at              TIMESTAMPTZ,
    check_out_lat             DOUBLE PRECISION,
    check_out_lng             DOUBLE PRECISION,
    check_out_accuracy_m      DOUBLE PRECISION,
    check_out_selfie_id       UUID REFERENCES photos (id),
    check_out_reason          TEXT,

    -- Permintaan pulang lebih awal.
    early_leave_requested_at  TIMESTAMPTZ,
    early_leave_reason        TEXT,

    created_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE (employee_id, work_date)
);

CREATE INDEX idx_attendances_work_date ON attendances (work_date);

-- Foto hasil kerja: satu record per absensi/sesi.
CREATE TABLE work_reports (
    id             UUID PRIMARY KEY,
    attendance_id  UUID        NOT NULL REFERENCES attendances (id) ON DELETE CASCADE,
    session_no     SMALLINT    NOT NULL,
    photo_id       UUID        NOT NULL REFERENCES photos (id),
    received_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (attendance_id, session_no)
);
