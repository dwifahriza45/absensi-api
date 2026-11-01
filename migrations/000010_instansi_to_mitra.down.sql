-- Restore the previous schema and code prefix, preserving UUIDs and relations.
BEGIN;
DROP TRIGGER mitra_public_id ON mitra;
ALTER TABLE employees DROP CONSTRAINT employees_mitra_id_fkey;
ALTER TABLE mitra DROP CONSTRAINT mitra_mitra_id_format;

UPDATE mitra SET mitra_id=regexp_replace(mitra_id,'^MTR-','INT-');
UPDATE employees SET mitra_id=regexp_replace(mitra_id,'^MTR-','INT-') WHERE mitra_id IS NOT NULL;
INSERT INTO public_id_counters(prefix,id_date,last_value)
SELECT 'INT',id_date,last_value FROM public_id_counters WHERE prefix='MTR'
ON CONFLICT(prefix,id_date) DO UPDATE SET last_value=GREATEST(public_id_counters.last_value,EXCLUDED.last_value);
DELETE FROM public_id_counters WHERE prefix='MTR';

ALTER TABLE mitra RENAME COLUMN mitra_id TO instansi_id;
ALTER TABLE employees RENAME COLUMN mitra_id TO instansi_id;
ALTER TABLE mitra RENAME TO instansi;
ALTER TABLE instansi RENAME CONSTRAINT mitra_pkey TO instansi_pkey;
ALTER TABLE instansi RENAME CONSTRAINT mitra_name_key TO instansi_name_key;
ALTER TABLE instansi RENAME CONSTRAINT mitra_name_check TO instansi_name_check;
ALTER TABLE instansi RENAME CONSTRAINT mitra_mitra_id_key TO instansi_instansi_id_key;
ALTER TABLE instansi ADD CONSTRAINT instansi_instansi_id_format CHECK (instansi_id ~ '^INT-[0-9]{8}-[0-9]{6}$');
ALTER INDEX idx_employees_mitra_id RENAME TO idx_employees_instansi_id;
ALTER TABLE employees ADD CONSTRAINT employees_instansi_id_fkey FOREIGN KEY(instansi_id)
    REFERENCES instansi(instansi_id) ON UPDATE RESTRICT ON DELETE RESTRICT;
CREATE TRIGGER instansi_public_id BEFORE INSERT ON instansi
    FOR EACH ROW EXECUTE FUNCTION assign_public_id('instansi_id','INT','created_at','name');
COMMIT;
