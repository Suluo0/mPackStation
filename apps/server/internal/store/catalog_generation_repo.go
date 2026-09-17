package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

// PublishCatalogGeneration mirrors the compatibility catalog into an immutable
// pack generation and atomically advances the pack pointer after checking the
// configuration revision.
func (r *Repository) PublishCatalogGeneration(ctx context.Context, packID string, configRevision int64, catalog Catalog, resolverVersion, requestID string) error {
	return r.WithTx(ctx, func(tx *Repository) error {
		var current int64
		if err := tx.db.QueryRowContext(ctx, `SELECT config_revision FROM packs WHERE id=?`, packID).Scan(&current); err != nil {
			return err
		}
		if current != configRevision {
			return ErrConflict
		}
		generationID := NewScopedID("generation", packID, fmt.Sprintf("%d\x00%d\x00%d\x00%s", configRevision, catalog.Revision, catalog.BuiltAt, requestID))
		type sourceRun struct{ sourceID, runID string }
		rows, err := tx.db.QueryContext(ctx, `SELECT s.id,COALESCE((SELECT r.id FROM parse_runs r WHERE r.pack_id=s.pack_id AND r.source_id=s.id AND r.execution_status='succeeded' ORDER BY r.created_at DESC,r.id DESC LIMIT 1),'') FROM pack_content_sources s JOIN pack_mod_selections x ON x.pack_id=s.pack_id AND x.id=s.selection_id JOIN pack_mods pm ON pm.pack_id=x.pack_id AND pm.id=x.pack_mod_id AND pm.current_selection_id=x.id WHERE s.pack_id=? AND pm.status='installed' ORDER BY s.id`, packID)
		if err != nil {
			return err
		}
		var sources []sourceRun
		for rows.Next() {
			var source sourceRun
			if err := rows.Scan(&source.sourceID, &source.runID); err != nil {
				rows.Close()
				return err
			}
			sources = append(sources, source)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		digest := sha256.New()
		completeness := "complete"
		for _, source := range sources {
			_, _ = digest.Write([]byte(source.sourceID + "\x00" + source.runID + "\x00"))
			if source.runID == "" {
				completeness = "partial"
			}
		}
		inputSHA := hex.EncodeToString(digest.Sum(nil))
		diagnostics, _ := json.Marshal(map[string]any{"warnings": catalog.Warnings})
		if _, err := tx.db.ExecContext(ctx, `INSERT INTO catalog_generations(pack_id,id,config_revision,input_sha256,resolver_version,status,completeness,started_at,finished_at,diagnostics) VALUES(?,?,?,?,?,'building',?,?,?,?)`, packID, generationID, configRevision, inputSHA, resolverVersion, completeness, catalog.BuiltAt, catalog.BuiltAt, string(diagnostics)); err != nil {
			return err
		}
		for priority, source := range sources {
			condition := "true"
			status := "included"
			var run any = source.runID
			if source.runID == "" {
				condition, status, run = "unknown", "pending", nil
			}
			if _, err := tx.db.ExecContext(ctx, `INSERT INTO catalog_generation_sources(pack_id,generation_id,source_id,run_id,inclusion_status,priority,condition_result) VALUES(?,?,?,?,?,?,?)`, packID, generationID, source.sourceID, run, status, priority, condition); err != nil {
				return err
			}
		}
		entryKeys := map[string]string{}
		ensureEntry := func(kind, registry, resourceKey, availability string) (string, error) {
			mapKey := kind + "\x00" + registry + "\x00" + resourceKey
			if id := entryKeys[mapKey]; id != "" {
				return id, nil
			}
			keyID := NewScopedID("key", packID, mapKey)
			if _, err := tx.db.ExecContext(ctx, `INSERT OR IGNORE INTO content_keys(pack_id,id,kind,registry,resource_key,qualifier) VALUES(?,?,?,?,?,'')`, packID, keyID, kind, registry, resourceKey); err != nil {
				return "", err
			}
			var winner string
			_ = tx.db.QueryRowContext(ctx, `SELECT d.id FROM content_definitions d JOIN parse_runs r ON r.pack_id=d.pack_id AND r.id=d.run_id JOIN catalog_generation_sources gs ON gs.pack_id=r.pack_id AND gs.generation_id=? AND gs.run_id=r.id WHERE d.pack_id=? AND d.key_id=? AND d.parse_status IN('parsed','partial') ORDER BY r.created_at DESC,d.id DESC LIMIT 1`, generationID, packID, keyID).Scan(&winner)
			var winnerValue any
			if winner != "" {
				winnerValue = winner
			}
			if _, err := tx.db.ExecContext(ctx, `INSERT INTO catalog_entries(pack_id,generation_id,key_id,winner_definition_id,availability,completeness) VALUES(?,?,?,?,?,?)`, packID, generationID, keyID, winnerValue, availability, completeness); err != nil {
				return "", err
			}
			if winner != "" {
				if _, err := tx.db.ExecContext(ctx, `INSERT INTO catalog_definition_decisions(pack_id,generation_id,definition_id,key_id,decision,precedence,condition_result,reason) VALUES(?,?,?,?,'selected',0,'true','current source order')`, packID, generationID, winner, keyID); err != nil {
					return "", err
				}
			}
			entryKeys[mapKey] = keyID
			return keyID, nil
		}
		for _, item := range catalog.Items {
			keyID, err := ensureEntry("item_model", "item", item.ID, "available")
			if err != nil {
				return err
			}
			for _, name := range item.Names {
				if _, err := tx.db.ExecContext(ctx, `INSERT INTO catalog_text_resolutions(pack_id,generation_id,key_id,role,requested_locale,text,resolved_locale,fallback_kind) VALUES(?,?,?,'display_name',?,?,?,'translation')`, packID, generationID, keyID, name.Locale, name.Name, name.Locale); err != nil {
					return err
				}
			}
		}
		for _, block := range catalog.Blocks {
			if _, err := ensureEntry("item_model", "block", block.ID, "available"); err != nil {
				return err
			}
		}
		for _, recipe := range catalog.Recipes {
			if _, err := ensureEntry("recipe", "recipe", recipe.ID, mapRecipeAvailability(recipe.Status)); err != nil {
				return err
			}
		}
		for _, tag := range catalog.Tags {
			tagKey, err := ensureEntry("tag", tag.Registry, tag.ID, mapTagAvailability(tag.Status))
			if err != nil {
				return err
			}
			var tagDefinition string
			_ = tx.db.QueryRowContext(ctx, `SELECT COALESCE(winner_definition_id,'') FROM catalog_entries WHERE pack_id=? AND generation_id=? AND key_id=?`, packID, generationID, tagKey).Scan(&tagDefinition)
			members := append([]string(nil), tag.Members...)
			sort.Strings(members)
			for _, member := range members {
				kind := "item_model"
				memberKey, err := ensureEntry(kind, tag.Registry, member, "available")
				if err != nil {
					return err
				}
				if _, err := tx.db.ExecContext(ctx, `INSERT OR IGNORE INTO catalog_tag_members(pack_id,generation_id,tag_key_id,member_key_id,certainty) VALUES(?,?,?,?,?)`, packID, generationID, tagKey, memberKey, "confirmed"); err != nil {
					return err
				}
				if tagDefinition != "" {
					evidenceID := NewScopedID("tag-evidence", packID, generationID+"\x00"+tag.ID+"\x00"+member)
					if _, err := tx.db.ExecContext(ctx, `INSERT OR IGNORE INTO catalog_tag_member_evidence(pack_id,generation_id,id,tag_key_id,member_key_id,definition_id,entry_ordinal) VALUES(?,?,?,?,?,?,0)`, packID, generationID, evidenceID, tagKey, memberKey, tagDefinition); err != nil {
						return err
					}
				}
			}
			for index, diagnostic := range tag.Diagnostics {
				issueID := NewScopedID("tag-issue", packID, generationID+"\x00"+tag.ID+fmt.Sprint(index))
				detail, _ := json.Marshal(map[string]string{"message": diagnostic})
				if _, err := tx.db.ExecContext(ctx, `INSERT INTO catalog_tag_issues(pack_id,generation_id,id,tag_key_id,definition_id,code,detail) VALUES(?,?,?,?,?,'resolution_warning',?)`, packID, generationID, issueID, tagKey, nullableText(tagDefinition), string(detail)); err != nil {
					return err
				}
			}
		}
		for _, icon := range catalog.Icons {
			keyID := entryKeys["item_model\x00item\x00"+icon.ItemID]
			if keyID == "" {
				continue
			}
			sha := sha256.Sum256(icon.Data)
			shaHex := hex.EncodeToString(sha[:])
			fileID := NewGlobalID("file", shaHex)
			if err := tx.registerScopedFile(ctx, PackScopedFile{ID: fileID, SHA256: shaHex, SizeBytes: int64(len(icon.Data)), MediaType: icon.Mime, Verified: true}, catalog.BuiltAt); err != nil {
				return err
			}
			if _, err := tx.db.ExecContext(ctx, `INSERT INTO catalog_icons(pack_id,generation_id,key_id,view_kind,status,file_id,width,height,mime,source_kind,renderer_version,input_sha256) VALUES(?,?,?,'inventory','ready',?,?,?,?,?,'item-icon-v2',?)`, packID, generationID, keyID, fileID, icon.Width, icon.Height, icon.Mime, icon.Source, shaHex); err != nil {
				return err
			}
			var definitionID string
			_ = tx.db.QueryRowContext(ctx, `SELECT COALESCE(winner_definition_id,'') FROM catalog_entries WHERE pack_id=? AND generation_id=? AND key_id=?`, packID, generationID, keyID).Scan(&definitionID)
			if definitionID != "" {
				if _, err := tx.db.ExecContext(ctx, `INSERT OR IGNORE INTO catalog_icon_inputs(pack_id,generation_id,key_id,view_kind,definition_id,role) VALUES(?,?,?,'inventory',?,'model')`, packID, generationID, keyID, definitionID); err != nil {
					return err
				}
			}
		}
		if _, err := tx.db.ExecContext(ctx, `UPDATE catalog_generations SET status='superseded' WHERE pack_id=? AND status='ready' AND id<>?`, packID, generationID); err != nil {
			return err
		}
		if _, err := tx.db.ExecContext(ctx, `UPDATE catalog_generations SET status='ready' WHERE pack_id=? AND id=?`, packID, generationID); err != nil {
			return err
		}
		if _, err := tx.db.ExecContext(ctx, `UPDATE packs SET current_generation_id=? WHERE id=? AND config_revision=?`, generationID, packID, configRevision); err != nil {
			return err
		}
		return nil
	})
}

func nullableText(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func mapRecipeAvailability(status string) string {
	switch status {
	case "unsupported":
		return "unsupported"
	case "invalid":
		return "invalid"
	case "partial":
		return "dynamic"
	}
	return "available"
}

func mapTagAvailability(status string) string {
	switch status {
	case "missing":
		return "missing"
	case "invalid":
		return "invalid"
	case "partial":
		return "dynamic"
	}
	return "available"
}
