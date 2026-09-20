-- Fix historical misclassification: advancement/recipes/** was stored as kind=recipe
-- because the extractor matched Contains("/recipes/") before the advancement folder.
-- Real crafting recipes live under data/<ns>/recipe(s)/.

UPDATE mod_content
SET kind = 'advancement'
WHERE kind = 'recipe'
  AND (
    path LIKE '%/advancement/recipes/%'
    OR path LIKE '%/advancement/%'
    OR path LIKE '%/advancements/%'
  );
