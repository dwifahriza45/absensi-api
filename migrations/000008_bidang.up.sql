-- 000008: Normalisasi bidang menjadi master dengan ID bisnis BID.
BEGIN;

CREATE TABLE bidang (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    bidang_id   TEXT        NOT NULL UNIQUE CHECK (bidang_id ~ '^BID-[0-9]{8}-[0-9]{6}$'),
    name        TEXT        NOT NULL UNIQUE CHECK (btrim(name) <> ''),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TRIGGER bidang_public_id
    BEFORE INSERT ON bidang
    FOR EACH ROW EXECUTE FUNCTION assign_public_id('bidang_id', 'BID', 'created_at', 'name');

-- Gabungkan nama dengan spasi luar yang berbeda. Nilai kosong tetap tanpa bidang.
INSERT INTO bidang(name)
SELECT DISTINCT btrim(bidang) FROM employees WHERE btrim(bidang) <> '' ORDER BY 1;

ALTER TABLE employees ADD COLUMN bidang_id TEXT
    CONSTRAINT employees_bidang_id_fkey REFERENCES bidang(bidang_id) ON UPDATE RESTRICT ON DELETE RESTRICT;
UPDATE employees e SET bidang_id = b.bidang_id FROM bidang b WHERE b.name = btrim(e.bidang);

DROP INDEX idx_employees_filters;
ALTER TABLE employees DROP COLUMN bidang;
CREATE INDEX idx_employees_bidang_id ON employees(bidang_id);
CREATE INDEX idx_employees_filters ON employees(instansi_id, daerah, bidang_id) WHERE is_active;

COMMIT;
