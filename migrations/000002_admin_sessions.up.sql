-- 000002: Akun admin terpisah dari employees, dengan sesi login bersama.

-- Akun admin: autentikasi email + password bcrypt.
CREATE TABLE users (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    fullname       TEXT        NOT NULL,
    email          TEXT        NOT NULL UNIQUE,
    password_hash  TEXT        NOT NULL,
    role           TEXT        NOT NULL CHECK (role = 'admin'),
    active         BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Akun awal untuk development. Ganti password sebelum production.
-- Startup production menolak password bawaan ini.
INSERT INTO users (fullname, email, password_hash, role)
VALUES (
    'Admin',
    'admin@absensi.prod',
    crypt('Admin123!', gen_salt('bf', 10)),
    'admin'
);

-- Subject berisi UUID users untuk admin, atau UUID employees untuk karyawan.
-- Karena mengacu ke dua tabel, subject tidak memakai satu foreign key langsung.
-- Refresh token disimpan sebagai hash, bukan token mentah.
CREATE TABLE sessions (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    subject       UUID        NOT NULL,
    role          TEXT        NOT NULL CHECK (role IN ('admin', 'karyawan')),
    refresh_hash  TEXT        NOT NULL UNIQUE,
    expires_at    TIMESTAMPTZ NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_sessions_expiry ON sessions (expires_at);
