-- Nomor kontak terenkripsi untuk admin; hash tetap untuk login dan uniqueness.
-- Nomor dari record lama tidak dapat dipulihkan dari hash: isi ulang/import.
ALTER TABLE employees ADD COLUMN phone_encrypted TEXT NOT NULL DEFAULT '';
