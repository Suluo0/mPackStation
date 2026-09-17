"""把 pakku export 产出的 mrpack 转成"全下载链接"版（PCL2 风格）。

pakku 遵守 Modrinth 上传规范，会把 CurseForge 来源的模组以 jar 内嵌进 overrides。
本脚本把这些 jar 还原为 manifest 里的 forgecdn 下载链接，产出 *-all-links.mrpack。

数据来源：
- pakku-lock.json 里 curseforge 类型的 file 条目（url + sha1）
- manual-mods.json（.pakku/overrides/mods/ 手工模组的 url + sha1/sha512）
- 内嵌 jar 的 sha512 在运行时从 zip 内计算

用法: python tools/export-mrpack-all-links.py   （在 mc_dev/ 下运行）
"""
import hashlib
import json
import sys
import zipfile
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
MODPACK = ROOT / "modpack"

def main() -> int:
    mrpacks = sorted(
        p for p in (MODPACK / "build" / "modrinth").glob("*.mrpack")
        if not p.stem.endswith("-all-links")
    )
    if not mrpacks:
        print("没有找到 build/modrinth/*.mrpack，先运行 pakku export")
        return 1
    src = mrpacks[0]
    lock = json.loads((MODPACK / "pakku-lock.json").read_text(encoding="utf-8"))
    manual = json.loads((MODPACK / "manual-mods.json").read_text(encoding="utf-8"))

    # 文件名 -> 下载信息（curseforge 托管项目 + 手工模组）
    cf_files = {}
    for p in lock["projects"]:
        for f in p.get("files", []):
            if f.get("type") == "curseforge":
                cf_files[f["file_name"]] = {
                    "url": f["url"], "sha1": f["hashes"]["sha1"], "size": f["size"],
                }

    with zipfile.ZipFile(src) as z:
        index = json.loads(z.read("modrinth.index.json"))
        names = z.namelist()
        embedded = [n for n in names if n.startswith("overrides/mods/") and n.endswith(".jar")]
        if not embedded:
            print("没有内嵌 jar，无需转换")
            return 0
        existing = {f["path"] for f in index["files"]}
        added = 0
        for n in embedded:
            fn = n.split("/")[-1]
            path = "mods/" + fn
            if path in existing:
                continue
            meta = manual.get(fn) or cf_files.get(fn)
            if not meta:
                print(f"!! 找不到 {fn} 的下载信息，保留内嵌")
                continue
            data = z.read(n)
            sha512 = meta.get("sha512") or hashlib.sha512(data).hexdigest()
            if len(data) != meta["size"]:
                print(f"!! {fn} 大小与记录不符，保留内嵌")
                continue
            index["files"].append({
                "path": path,
                "hashes": {"sha1": meta["sha1"], "sha512": sha512},
                "downloads": [meta["url"]],
                "fileSize": meta["size"],
            })
            added += 1

        dst = src.with_name(src.stem + "-all-links.mrpack")
        skip = {"modrinth.index.json"} | {
            n for n in embedded if "mods/" + n.split("/")[-1] in {f["path"] for f in index["files"][-added:]}
        }
        with zipfile.ZipFile(dst, "w", zipfile.ZIP_DEFLATED) as out:
            out.writestr("modrinth.index.json", json.dumps(index, ensure_ascii=False, indent=1))
            for n in names:
                if n in skip or n.endswith("/"):
                    continue
                out.writestr(n, z.read(n))

    print(f"manifest {len(index['files'])} 项，{added} 个 jar 转为下载链接 -> {dst}")
    return 0

if __name__ == "__main__":
    sys.exit(main())
