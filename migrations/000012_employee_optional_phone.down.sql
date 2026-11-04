-- Downgrade requires contacts to be filled for employees without a phone.
ALTER TABLE employees ALTER COLUMN phone_hash SET NOT NULL;
