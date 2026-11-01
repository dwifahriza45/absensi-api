-- Rollback menghapus nomor terenkripsi; hash dan login tetap dipertahankan.
ALTER TABLE employees DROP COLUMN phone_encrypted;
