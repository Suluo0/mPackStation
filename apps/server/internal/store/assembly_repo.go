package store

import (
	"context"
	"database/sql"
)

// PackAssemblySource is one authoritative mod entry the builder needs to write a
// real modrinth.index.json.
//
// 权威来源是「包的当前选中项」而不是调用方传入的文件：pack_mods.current_selection_id
// → pack_mod_selections → selection_platform_pins(role='primary') → platform_release_files。
// 这条链已经在添加模组/锁依赖时写好了（见 pack_scoped_repo.go 的写入侧），构建只是读。
//
// DownloadURL 为空表示这个模组在本机只是本地 jar（acquisition='local'），
// 它没有可供安装方下载的地址：装配必须把它作为阻塞项报出来，
// 而不是静默产出一个「看起来成功、其实缺模组」的包。
type PackAssemblySource struct {
	ModID       string
	DisplayName string
	FileName    string
	DownloadURL string
	SHA1        string
	SHA256      string
	SHA512      string
	SizeBytes   int64
	Acquisition string
	Status      string // installed/pending/disabled（removed 与内建 minecraft 已在 SQL 侧排除）
}

// ListPackAssemblySources returns the pack's mod清单 in shipping order.
// LEFT JOIN 而不是 JOIN：没有选中项的模组（只有本机 jar 的本地模组）必须仍然出现，
// 下载地址为空 → 上层按「无法解析来源」阻止构建。用 INNER JOIN 会让它从结果里消失，
// 于是"构建成功但包里没有它"——正是本函数注释里声明要禁止的静默缺件。
// 口径与 ListPackMods 一致：排除 status='removed' 与 origin='builtin'
// （内建的 minecraft 行走 dependencies 字段，不进 files[]）。
func (r *Repository) ListPackAssemblySources(ctx context.Context, packID string) ([]PackAssemblySource, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT COALESCE(pm.mod_id,''), COALESCE(NULLIF(m.display_name,''), pm.display_name, ''),
		       COALESCE(NULLIF(rf.file_name,''), pm.file_name, ''), COALESCE(rf.download_url,''),
		       COALESCE(json_extract(rf.expected_hashes,'$.sha1'),''),
		       COALESCE(json_extract(rf.expected_hashes,'$.sha256'),''),
		       COALESCE(json_extract(rf.expected_hashes,'$.sha512'),''),
		       COALESCE(rf.expected_size,0), COALESCE(s.acquisition,''), pm.status
		FROM pack_mods pm
		LEFT JOIN pack_mod_selections s ON s.pack_id = pm.pack_id AND s.id = pm.current_selection_id
		LEFT JOIN mods m ON m.mod_id = pm.mod_id
		LEFT JOIN selection_platform_pins p ON p.pack_id = pm.pack_id AND p.selection_id = s.id AND p.role = 'primary'
		LEFT JOIN platform_release_files rf ON rf.id = p.release_file_id
		WHERE pm.pack_id = ? AND pm.status <> 'removed' AND pm.origin <> 'builtin'
		ORDER BY COALESCE(NULLIF(m.display_name,''), pm.display_name), pm.id`, packID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]PackAssemblySource, 0, 8)
	for rows.Next() {
		var s PackAssemblySource
		var size sql.NullInt64
		if err := rows.Scan(&s.ModID, &s.DisplayName, &s.FileName, &s.DownloadURL, &s.SHA1, &s.SHA256, &s.SHA512, &size, &s.Acquisition, &s.Status); err != nil {
			return nil, err
		}
		s.SizeBytes = size.Int64
		out = append(out, s)
	}
	return out, rows.Err()
}
