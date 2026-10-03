package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// CatalogName is one locale-specific display name with traceable provenance.
type CatalogName struct {
	Locale string `json:"locale"`
	Name   string `json:"name"`
	Key    string `json:"key"`
	Source string `json:"source"`
}
type CatalogItem struct {
	ID, Evidence, Source, ModelPath, IconStatus string
	IconReason                                  string
	Names                                       []CatalogName
	Tags                                        []string
}
type CatalogIcon struct {
	ItemID, Mime, Source string
	Data                 []byte
	Width, Height        int
}
type CatalogBlock struct {
	ID, Evidence, Source, BlockstatePath string
	Names                                []CatalogName
	Tags, ItemIDs                        []string
}
type CatalogDefinition struct {
	Source, Path        string
	Payload, Conditions json.RawMessage
	Replace             bool
}
type CatalogTagEntry struct {
	Definition, Position int
	TargetKind, TargetID string
	Required             bool
	Operation            string
}
type CatalogTag struct {
	Registry, ID, Status string
	Diagnostics          []string
	Names                []CatalogName
	Definitions          []CatalogDefinition
	Entries              []CatalogTagEntry
	Members              []string
}
type CatalogRecipeRef struct {
	Role        string `json:"role"`
	Kind        string `json:"kind"`
	ID          string `json:"id"`
	Slot        int    `json:"slot"`
	Alternative int    `json:"alternative"`
	Count       int    `json:"count"`
}
type CatalogRecipe struct {
	ID, Type, Source, Path, Status string
	Payload                        json.RawMessage
	Diagnostics                    []string
	Refs                           []CatalogRecipeRef
}
type Catalog struct {
	Revision, BuiltAt int64
	Warnings          []string
	Items             []CatalogItem
	Icons             []CatalogIcon
	Blocks            []CatalogBlock
	Tags              []CatalogTag
	Recipes           []CatalogRecipe
}
type CatalogSource struct {
	Pack     PackRecord
	Revision int64
	Mods     []PackModRecord
	Runs     []ModContentRunRecord
	Content  []ModContentRecord
}

type CatalogState struct {
	SourceRevision, BuiltRevision, BuiltAt int64
	Status, LastError                      string
	Warnings                               []string
}

// ReadCatalogState reports initialization progress without requiring a complete catalog.
func (r *Repository) ReadCatalogState(ctx context.Context, packID string) (CatalogState, error) {
	var state CatalogState
	var warnings string
	err := r.db.QueryRowContext(ctx, `SELECT source_revision,built_revision,build_status,built_at,last_error,warnings FROM pack_catalog_state WHERE pack_id=?`, packID).Scan(&state.SourceRevision, &state.BuiltRevision, &state.Status, &state.BuiltAt, &state.LastError, &warnings)
	if errors.Is(err, sql.ErrNoRows) {
		return state, ErrNotFound
	}
	if err != nil {
		return state, err
	}
	if err = json.Unmarshal([]byte(warnings), &state.Warnings); err != nil {
		return state, err
	}
	return state, nil
}

// SetCatalogBuildState records lifecycle without touching the last complete catalog.
func (r *Repository) SetCatalogBuildState(ctx context.Context, packID, status, message string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE pack_catalog_state SET build_status=?,last_error=? WHERE pack_id=?`, status, message, packID)
	return err
}

// CatalogSourceSnapshot reads all inputs at one revision before archive parsing begins.
func (r *Repository) CatalogSourceSnapshot(ctx context.Context, packID string) (s CatalogSource, err error) {
	err = r.WithTx(ctx, func(tx *Repository) error {
		var e error
		s.Pack, e = tx.GetPack(ctx, packID)
		if e != nil {
			return e
		}
		if e = tx.db.QueryRowContext(ctx, `SELECT source_revision FROM pack_catalog_state WHERE pack_id=?`, packID).Scan(&s.Revision); e != nil {
			return e
		}
		s.Mods, e = tx.ListPackMembers(ctx, packID)
		if e != nil {
			return e
		}
		for _, m := range s.Mods {
			run, runErr := tx.GetModContentRunBySHA(ctx, m.ID, m.SHA1)
			if runErr != nil && !errors.Is(runErr, ErrNotFound) {
				return runErr
			}
			s.Runs = append(s.Runs, run)
			cursor := ""
			for {
				rows, next, _, listErr := tx.ListModContent(ctx, packID, m.ID, "", 500, cursor)
				if listErr != nil {
					return listErr
				}
				for _, row := range rows {
					if row.Kind == "item_model" || row.Kind == "texture" || row.Kind == "lang" || row.Kind == "tag" || row.Kind == "recipe" {
						s.Content = append(s.Content, row)
					}
				}
				if next == "" {
					break
				}
				cursor = next
			}
		}
		return nil
	})
	return
}

// ReplaceCatalog atomically swaps a derived catalog if its source revision is still current.
func (r *Repository) ReplaceCatalog(ctx context.Context, packID string, c Catalog, requestID string) error {
	return r.WithTx(ctx, func(tx *Repository) error {
		var revision int64
		if err := tx.db.QueryRowContext(ctx, `SELECT source_revision FROM pack_catalog_state WHERE pack_id=?`, packID).Scan(&revision); err != nil {
			return err
		}
		if revision != c.Revision {
			return ErrConflict
		}
		for _, q := range []string{`DELETE FROM pack_catalog_recipes WHERE pack_id=?`, `DELETE FROM pack_catalog_tags WHERE pack_id=?`, `DELETE FROM pack_catalog_items WHERE pack_id=?`, `DELETE FROM pack_catalog_blocks WHERE pack_id=?`} {
			if _, err := tx.db.ExecContext(ctx, q, packID); err != nil {
				return err
			}
		}
		for _, it := range c.Items {
			if _, err := tx.db.ExecContext(ctx, `INSERT INTO pack_catalog_items(pack_id,item_id,evidence,source,model_path,icon_status,icon_reason) VALUES(?,?,?,?,?,?,?)`, packID, it.ID, it.Evidence, it.Source, it.ModelPath, it.IconStatus, it.IconReason); err != nil {
				return err
			}
			for _, n := range it.Names {
				if _, err := tx.db.ExecContext(ctx, `INSERT INTO pack_catalog_item_names VALUES(?,?,?,?,?,?)`, packID, it.ID, n.Locale, n.Name, n.Key, n.Source); err != nil {
					return err
				}
			}
		}
		for _, icon := range c.Icons {
			if _, err := tx.db.ExecContext(ctx, `INSERT INTO pack_catalog_item_icons VALUES(?,?,?,?,?,?,?)`, packID, icon.ItemID, icon.Mime, icon.Data, icon.Width, icon.Height, icon.Source); err != nil {
				return err
			}
		}
		for _, block := range c.Blocks {
			if _, err := tx.db.ExecContext(ctx, `INSERT INTO pack_catalog_blocks VALUES(?,?,?,?,?)`, packID, block.ID, block.Evidence, block.Source, block.BlockstatePath); err != nil {
				return err
			}
			for _, n := range block.Names {
				if _, err := tx.db.ExecContext(ctx, `INSERT INTO pack_catalog_block_names VALUES(?,?,?,?,?,?)`, packID, block.ID, n.Locale, n.Name, n.Key, n.Source); err != nil {
					return err
				}
			}
			for _, itemID := range block.ItemIDs {
				if _, err := tx.db.ExecContext(ctx, `INSERT INTO pack_catalog_item_blocks VALUES(?,?,?)`, packID, itemID, block.ID); err != nil {
					return err
				}
			}
		}
		for _, tag := range c.Tags {
			diagnostics, _ := json.Marshal(tag.Diagnostics)
			if _, err := tx.db.ExecContext(ctx, `INSERT INTO pack_catalog_tags VALUES(?,?,?,?,?)`, packID, tag.Registry, tag.ID, tag.Status, string(diagnostics)); err != nil {
				return err
			}
			for _, n := range tag.Names {
				if _, err := tx.db.ExecContext(ctx, `INSERT INTO pack_catalog_tag_names VALUES(?,?,?,?,?,?,?)`, packID, tag.Registry, tag.ID, n.Locale, n.Name, n.Key, n.Source); err != nil {
					return err
				}
			}
			for i, d := range tag.Definitions {
				conditions := d.Conditions
				if len(conditions) == 0 {
					conditions = json.RawMessage(`[]`)
				}
				if _, err := tx.db.ExecContext(ctx, `INSERT INTO pack_catalog_tag_definitions VALUES(?,?,?,?,?,?,?,?,?)`, packID, tag.Registry, tag.ID, i, d.Source, d.Path, string(d.Payload), boolInt(d.Replace), string(conditions)); err != nil {
					return err
				}
			}
			for _, entry := range tag.Entries {
				if _, err := tx.db.ExecContext(ctx, `INSERT INTO pack_catalog_tag_entries VALUES(?,?,?,?,?,?,?,?,?)`, packID, tag.Registry, tag.ID, entry.Definition, entry.Position, entry.TargetKind, entry.TargetID, catalogBool(entry.Required), entry.Operation); err != nil {
					return err
				}
			}
			for _, member := range tag.Members {
				if _, err := tx.db.ExecContext(ctx, `INSERT INTO pack_catalog_tag_members VALUES(?,?,?,?)`, packID, tag.Registry, tag.ID, member); err != nil {
					return err
				}
			}
		}
		for _, recipe := range c.Recipes {
			diagnostics, _ := json.Marshal(recipe.Diagnostics)
			if _, err := tx.db.ExecContext(ctx, `INSERT INTO pack_catalog_recipes VALUES(?,?,?,?,?,?,?,?)`, packID, recipe.ID, recipe.Type, recipe.Source, recipe.Path, string(recipe.Payload), recipe.Status, string(diagnostics)); err != nil {
				return err
			}
			for _, ref := range recipe.Refs {
				if _, err := tx.db.ExecContext(ctx, `INSERT INTO pack_catalog_recipe_refs VALUES(?,?,?,?,?,?,?,?)`, packID, recipe.ID, ref.Role, ref.Slot, ref.Alternative, ref.Kind, ref.ID, ref.Count); err != nil {
					return err
				}
			}
		}
		warnings, _ := json.Marshal(c.Warnings)
		if _, err := tx.db.ExecContext(ctx, `UPDATE pack_catalog_state SET built_revision=?,build_status='succeeded',built_at=?,last_error='',warnings=? WHERE pack_id=?`, c.Revision, c.BuiltAt, string(warnings), packID); err != nil {
			return err
		}
		return addP6Evidence(ctx, tx, packID, requestID, c.BuiltAt, "content", "catalog.rebuild", "pack", packID, "catalog.rebuilt", map[string]any{"revision": c.Revision, "items": len(c.Items), "blocks": len(c.Blocks), "tags": len(c.Tags), "recipes": len(c.Recipes)})
	})
}

// ReadCatalogIcon returns one generated icon only when the containing catalog is current.
func (r *Repository) ReadCatalogIcon(ctx context.Context, packID, itemID string) (CatalogIcon, error) {
	var icon CatalogIcon
	current, err := r.currentCatalogGeneration(ctx, packID)
	if err != nil {
		return icon, err
	}
	if !current {
		return icon, r.catalogGenerationProblem(ctx, packID)
	}
	var source, built int64
	err = r.db.QueryRowContext(ctx, `SELECT s.source_revision,s.built_revision,i.item_id,i.mime,i.data,i.width,i.height,i.source FROM pack_catalog_state s JOIN pack_catalog_item_icons i ON i.pack_id=s.pack_id WHERE s.pack_id=? AND i.item_id=?`, packID, itemID).Scan(&source, &built, &icon.ItemID, &icon.Mime, &icon.Data, &icon.Width, &icon.Height, &icon.Source)
	if errors.Is(err, sql.ErrNoRows) {
		return icon, ErrNotFound
	}
	if err != nil {
		return icon, err
	}
	if source != built {
		return icon, ErrConflict
	}
	return icon, nil
}

func catalogBool(v bool) int {
	if v {
		return 1
	}
	return 0
}

// ReadCatalog returns only a fully built current revision and reconstructs both tag directions.
func (r *Repository) ReadCatalog(ctx context.Context, packID string) (c Catalog, err error) {
	c.Items, c.Blocks, c.Tags, c.Recipes, c.Warnings = []CatalogItem{}, []CatalogBlock{}, []CatalogTag{}, []CatalogRecipe{}, []string{}
	err = r.WithTx(ctx, func(tx *Repository) error {
		current, e := tx.currentCatalogGeneration(ctx, packID)
		if e != nil {
			return e
		}
		if !current {
			return tx.catalogGenerationProblem(ctx, packID)
		}
		var source int64
		var warnings string
		e = tx.db.QueryRowContext(ctx, `SELECT source_revision,built_revision,built_at,warnings FROM pack_catalog_state WHERE pack_id=?`, packID).Scan(&source, &c.Revision, &c.BuiltAt, &warnings)
		if errors.Is(e, sql.ErrNoRows) {
			return ErrNotFound
		}
		if e != nil {
			return e
		}
		if source != c.Revision {
			return ErrConflict
		}
		if e = json.Unmarshal([]byte(warnings), &c.Warnings); e != nil {
			return e
		}
		read := func(q string, visit func(*sql.Rows) error) error {
			rows, x := tx.db.QueryContext(ctx, q, packID)
			if x != nil {
				return x
			}
			defer rows.Close()
			for rows.Next() {
				if x = visit(rows); x != nil {
					return x
				}
			}
			return rows.Err()
		}
		items, blocks, tags, recipes := map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}
		if e = read(`SELECT item_id,evidence,source,model_path,icon_status FROM pack_catalog_items WHERE pack_id=? ORDER BY item_id`, func(rows *sql.Rows) error {
			var v CatalogItem
			v.Names = []CatalogName{}
			v.Tags = []string{}
			if x := rows.Scan(&v.ID, &v.Evidence, &v.Source, &v.ModelPath, &v.IconStatus); x != nil {
				return x
			}
			items[v.ID] = len(c.Items)
			c.Items = append(c.Items, v)
			return nil
		}); e != nil {
			return e
		}
		if e = read(`SELECT block_id,evidence,source,blockstate_path FROM pack_catalog_blocks WHERE pack_id=? ORDER BY block_id`, func(rows *sql.Rows) error {
			var v CatalogBlock
			v.Names = []CatalogName{}
			v.Tags = []string{}
			v.ItemIDs = []string{}
			if x := rows.Scan(&v.ID, &v.Evidence, &v.Source, &v.BlockstatePath); x != nil {
				return x
			}
			blocks[v.ID] = len(c.Blocks)
			c.Blocks = append(c.Blocks, v)
			return nil
		}); e != nil {
			return e
		}
		if e = read(`SELECT registry,tag_id,status,diagnostics FROM pack_catalog_tags WHERE pack_id=? ORDER BY registry,tag_id`, func(rows *sql.Rows) error {
			var v CatalogTag
			var d string
			v.Names = []CatalogName{}
			v.Definitions = []CatalogDefinition{}
			v.Entries = []CatalogTagEntry{}
			v.Members = []string{}
			if x := rows.Scan(&v.Registry, &v.ID, &v.Status, &d); x != nil {
				return x
			}
			if x := json.Unmarshal([]byte(d), &v.Diagnostics); x != nil {
				return x
			}
			tags[v.Registry+"\x00"+v.ID] = len(c.Tags)
			c.Tags = append(c.Tags, v)
			return nil
		}); e != nil {
			return e
		}
		if e = read(`SELECT recipe_id,type,source,path,payload,parse_status,diagnostics FROM pack_catalog_recipes WHERE pack_id=? ORDER BY recipe_id`, func(rows *sql.Rows) error {
			var v CatalogRecipe
			var p, d string
			v.Refs = []CatalogRecipeRef{}
			if x := rows.Scan(&v.ID, &v.Type, &v.Source, &v.Path, &p, &v.Status, &d); x != nil {
				return x
			}
			v.Payload = json.RawMessage(p)
			if x := json.Unmarshal([]byte(d), &v.Diagnostics); x != nil {
				return x
			}
			recipes[v.ID] = len(c.Recipes)
			c.Recipes = append(c.Recipes, v)
			return nil
		}); e != nil {
			return e
		}
		if e = read(`SELECT item_id,locale,name,translation_key,source FROM pack_catalog_item_names WHERE pack_id=? ORDER BY item_id,locale`, func(rows *sql.Rows) error {
			var id string
			var n CatalogName
			if x := rows.Scan(&id, &n.Locale, &n.Name, &n.Key, &n.Source); x != nil {
				return x
			}
			c.Items[items[id]].Names = append(c.Items[items[id]].Names, n)
			return nil
		}); e != nil {
			return e
		}
		if e = read(`SELECT block_id,locale,name,translation_key,source FROM pack_catalog_block_names WHERE pack_id=? ORDER BY block_id,locale`, func(rows *sql.Rows) error {
			var id string
			var n CatalogName
			if x := rows.Scan(&id, &n.Locale, &n.Name, &n.Key, &n.Source); x != nil {
				return x
			}
			c.Blocks[blocks[id]].Names = append(c.Blocks[blocks[id]].Names, n)
			return nil
		}); e != nil {
			return e
		}
		if e = read(`SELECT item_id,block_id FROM pack_catalog_item_blocks WHERE pack_id=? ORDER BY item_id,block_id`, func(rows *sql.Rows) error {
			var itemID, blockID string
			if x := rows.Scan(&itemID, &blockID); x != nil {
				return x
			}
			c.Blocks[blocks[blockID]].ItemIDs = append(c.Blocks[blocks[blockID]].ItemIDs, itemID)
			return nil
		}); e != nil {
			return e
		}
		if e = read(`SELECT registry,tag_id,locale,name,translation_key,source FROM pack_catalog_tag_names WHERE pack_id=? ORDER BY registry,tag_id,locale`, func(rows *sql.Rows) error {
			var reg, id string
			var n CatalogName
			if x := rows.Scan(&reg, &id, &n.Locale, &n.Name, &n.Key, &n.Source); x != nil {
				return x
			}
			key := reg + "\x00" + id
			c.Tags[tags[key]].Names = append(c.Tags[tags[key]].Names, n)
			return nil
		}); e != nil {
			return e
		}
		if e = read(`SELECT registry,tag_id,source,path,payload FROM pack_catalog_tag_definitions WHERE pack_id=? ORDER BY registry,tag_id,ordinal`, func(rows *sql.Rows) error {
			var reg, id, p string
			var d CatalogDefinition
			if x := rows.Scan(&reg, &id, &d.Source, &d.Path, &p); x != nil {
				return x
			}
			d.Payload = json.RawMessage(p)
			key := reg + "\x00" + id
			c.Tags[tags[key]].Definitions = append(c.Tags[tags[key]].Definitions, d)
			return nil
		}); e != nil {
			return e
		}
		if e = read(`SELECT registry,tag_id,member_id FROM pack_catalog_tag_members WHERE pack_id=? ORDER BY registry,tag_id,member_id`, func(rows *sql.Rows) error {
			var reg, id, member string
			if x := rows.Scan(&reg, &id, &member); x != nil {
				return x
			}
			key := reg + "\x00" + id
			c.Tags[tags[key]].Members = append(c.Tags[tags[key]].Members, member)
			if reg == "item" {
				if i, ok := items[member]; ok {
					c.Items[i].Tags = append(c.Items[i].Tags, id)
				}
			} else if i, ok := blocks[member]; ok {
				c.Blocks[i].Tags = append(c.Blocks[i].Tags, id)
			}
			return nil
		}); e != nil {
			return e
		}
		return read(`SELECT recipe_id,role,slot,alternative,ref_kind,ref_id,count FROM pack_catalog_recipe_refs WHERE pack_id=? ORDER BY recipe_id,role,slot,alternative`, func(rows *sql.Rows) error {
			var id string
			var ref CatalogRecipeRef
			if x := rows.Scan(&id, &ref.Role, &ref.Slot, &ref.Alternative, &ref.Kind, &ref.ID, &ref.Count); x != nil {
				return x
			}
			c.Recipes[recipes[id]].Refs = append(c.Recipes[recipes[id]].Refs, ref)
			return nil
		})
	})
	return
}

// catalogGenerationProblem 区分目录不可读的三种原因：包不存在 / 从未构建 / 构建过但已过期。
//
// 调用方是在 currentCatalogGeneration 返回 false 之后进来的，所以这里只负责回答
// 「为什么不是当前代次」，把三种情况压成一个 ErrConflict 会让界面无法给出正确指引。
func (r *Repository) catalogGenerationProblem(ctx context.Context, packID string) error {
	var exists int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM packs WHERE id=?`, packID).Scan(&exists); err != nil {
		return fmt.Errorf("check pack for catalog: %w", err)
	}
	if exists == 0 {
		return ErrNotFound
	}
	var built int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pack_catalog_state WHERE pack_id=? AND built_revision>0`, packID).Scan(&built); err != nil {
		return fmt.Errorf("check catalog build state: %w", err)
	}
	if built == 0 {
		return ErrCatalogNotBuilt
	}
	return ErrConflict
}

func (r *Repository) currentCatalogGeneration(ctx context.Context, packID string) (bool, error) {
	var current int
	err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM packs p JOIN catalog_generations g ON g.pack_id=p.id AND g.id=p.current_generation_id WHERE p.id=? AND g.status='ready' AND g.config_revision=p.config_revision)`, packID).Scan(&current)
	return current != 0, err
}
