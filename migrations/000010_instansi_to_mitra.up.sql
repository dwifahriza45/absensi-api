-- Rename master and business references without recreating employee/history rows.
BEGIN;
DROP TRIGGER instansi_public_id ON instansi;
ALTER TABLE employees DROP CONSTRAINT employees_instansi_id_fkey;
ALTER TABLE instansi DROP CONSTRAINT instansi_instansi_id_format;

ALTER TABLE instansi RENAME TO mitra;
ALTER TABLE mitra RENAME COLUMN instansi_id TO mitra_id;
ALTER TABLE employees RENAME COLUMN instansi_id TO mitra_id;

-- Keep the original date/sequence. Both sides are updated before restoring FK.
UPDATE mitra SET mitra_id = regexp_replace(mitra_id, '^INT-', 'MTR-');
UPDATE employees SET mitra_id = regexp_replace(mitra_id, '^INT-', 'MTR-') WHERE mitra_id IS NOT NULL;

-- Preserve allocated numbers, including codes belonging to deleted records.
INSERT INTO public_id_counters(prefix,id_date,last_value)
SELECT 'MTR',id_date,last_value FROM public_id_counters WHERE prefix='INT'
ON CONFLICT(prefix,id_date) DO UPDATE SET last_value=GREATEST(public_id_counters.last_value,EXCLUDED.last_value);
DELETE FROM public_id_counters WHERE prefix='INT';

ALTER TABLE mitra RENAME CONSTRAINT instansi_pkey TO mitra_pkey;
ALTER TABLE mitra RENAME CONSTRAINT instansi_name_key TO mitra_name_key;
ALTER TABLE mitra RENAME CONSTRAINT instansi_name_check TO mitra_name_check;
ALTER TABLE mitra RENAME CONSTRAINT instansi_instansi_id_key TO mitra_mitra_id_key;
ALTER TABLE mitra ADD CONSTRAINT mitra_mitra_id_format CHECK (mitra_id ~ '^MTR-[0-9]{8}-[0-9]{6}$');
ALTER INDEX idx_employees_instansi_id RENAME TO idx_employees_mitra_id;
ALTER TABLE employees ADD CONSTRAINT employees_mitra_id_fkey FOREIGN KEY(mitra_id)
    REFERENCES mitra(mitra_id) ON UPDATE RESTRICT ON DELETE RESTRICT;
CREATE TRIGGER mitra_public_id BEFORE INSERT ON mitra
    FOR EACH ROW EXECUTE FUNCTION assign_public_id('mitra_id','MTR','created_at','name');
COMMIT;
