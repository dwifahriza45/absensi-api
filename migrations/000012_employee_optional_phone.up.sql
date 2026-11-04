-- NULL permits multiple employees without a contact while retaining uniqueness for supplied numbers.
ALTER TABLE employees ALTER COLUMN phone_hash DROP NOT NULL;
