package service

import (
	_ "embed"
	"encoding/json"

	"mpackstation/internal/store"
)

// 内置模组身份知识库: 随二进制分发(go:embed), 只读。运行时与用户库
// mod_identity 表两层叠加——查询时先用户库后基线; 新确认的配对只写用户库,
// 绝不回写本文件。基线条目必须来自真实平台核实, 不允许凭记忆填写。
//
//go:embed mod_identity_baseline.json
var modIdentityBaselineJSON []byte

type modIdentityBaselineEntry struct {
	MRProjectID string `json:"mr"`
	CFProjectID string `json:"cf"`
	Name        string `json:"name"`
}

// baselineModIdentities parses the embedded knowledge pack. A malformed asset
// is a build-time mistake, but runtime stays defensive and yields no baseline.
func baselineModIdentities() []store.ModIdentityRecord {
	var entries []modIdentityBaselineEntry
	if err := json.Unmarshal(modIdentityBaselineJSON, &entries); err != nil {
		return nil
	}
	out := make([]store.ModIdentityRecord, 0, len(entries))
	for _, e := range entries {
		if e.MRProjectID != "" && e.CFProjectID != "" {
			out = append(out, store.ModIdentityRecord{MRProjectID: e.MRProjectID, CFProjectID: e.CFProjectID, DisplayName: e.Name})
		}
	}
	return out
}
