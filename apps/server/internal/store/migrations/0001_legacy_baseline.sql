-- Historical marker for the v1 database that shipped with the prototype.
--
-- The first prototype used an inline schema.sql and recorded version=1.  This
-- migration intentionally has no schema side effects: it gives new databases
-- the same auditable starting point while allowing the runner to recognise an
-- existing v1 database and upgrade it in 0002.  Never edit this file after a
-- database has recorded its checksum; append a new migration instead.
SELECT 1;
