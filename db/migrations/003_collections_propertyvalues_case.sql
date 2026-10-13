-- 003_collections_propertyvalues_case.sql — repair a quoting mistake in 002.
-- 002 created CollectionItem.propertyValues unquoted, so PostgreSQL folded it
-- to lowercase "propertyvalues" on databases where the migration's own DDL
-- materialized the table (existing installs upgraded through the ledger).
-- schema.sql and the API contract use the quoted camelCase name. 002 is
-- recorded and immutable, so the repair ships as this guarded rename; on
-- databases built from the corrected schema.sql the column already has the
-- right name and this is a no-op.
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM pg_attribute
     WHERE attrelid='"CollectionItem"'::regclass
       AND attname='propertyvalues'
       AND NOT attisdropped
  ) THEN
    ALTER TABLE "CollectionItem" RENAME COLUMN propertyvalues TO "propertyValues";
  END IF;
END $$;
