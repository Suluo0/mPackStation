//! 镜像源管理：URL 重写 + 多源竞速降级
//!
//! 支持 Mojang 官方和 BMCLAPI 国内镜像。
//! auto 模式采用竞速策略：双源同时下载，谁先完成且校验通过用谁。

/// 镜像源选择
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Mirror {
    /// 仅使用 Mojang 官方源
    Mojang,
    /// 仅使用 BMCLAPI 国内镜像
    Bmclapi,
    /// 双源竞速（默认）
    Auto,
}

impl Mirror {
    /// 从字符串解析
    pub fn from_str(s: &str) -> Self {
        match s.to_lowercase().as_str() {
            "mojang" => Mirror::Mojang,
            "bmclapi" | "bmcl" => Mirror::Bmclapi,
            _ => Mirror::Auto,
        }
    }
}

/// BMCLAPI 域名
const BMCLAPI_HOST: &str = "https://bmclapi2.bangbang93.com";

/// 将 Mojang URL 重写为 BMCLAPI URL
///
/// 按文件类型分类重写：
/// - piston-meta.mojang.com → bmclapi2.bangbang93.com（版本元数据）
/// - piston-data.mojang.com → bmclapi2.bangbang93.com（client.jar）
/// - libraries.minecraft.net → bmclapi2.bangbang93.com/maven（支持库）
/// - resources.download.minecraft.net → bmclapi2.bangbang93.com/assets（资源文件）
/// - 第三方库（非 Mojang 域名）不重写
pub fn rewrite_to_bmclapi(url: &str) -> String {
    if url.contains("piston-meta.mojang.com") {
        url.replace("https://piston-meta.mojang.com", BMCLAPI_HOST)
    } else if url.contains("piston-data.mojang.com") {
        url.replace("https://piston-data.mojang.com", BMCLAPI_HOST)
    } else if url.contains("libraries.minecraft.net") {
        url.replace(
            "https://libraries.minecraft.net",
            &format!("{}/maven", BMCLAPI_HOST),
        )
    } else if url.contains("resources.download.minecraft.net") {
        url.replace(
            "https://resources.download.minecraft.net",
            &format!("{}/assets", BMCLAPI_HOST),
        )
    } else {
        // 第三方库不重写
        url.to_string()
    }
}

/// 根据镜像模式获取下载 URL 列表
///
/// - Mojang: 返回 [original]
/// - Bmclapi: 返回 [bmclapi]（仅使用 BMCLAPI）
/// - Auto: 返回 [bmclapi, original]（BMCLAPI 优先，竞速）
pub fn get_download_urls(url: &str, mirror: Mirror) -> Vec<String> {
    let bmclapi_url = rewrite_to_bmclapi(url);
    let is_mojang_url = url.contains("mojang.com")
        || url.contains("minecraft.net")
        || url.contains("libraries.minecraft.net");

    match mirror {
        Mirror::Mojang => vec![url.to_string()],
        Mirror::Bmclapi => {
            if is_mojang_url && bmclapi_url != url {
                vec![bmclapi_url]
            } else {
                vec![url.to_string()]
            }
        }
        Mirror::Auto => {
            if is_mojang_url && bmclapi_url != url {
                vec![bmclapi_url, url.to_string()]
            } else {
                vec![url.to_string()]
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_rewrite_piston_meta() {
        let url = "https://piston-meta.mojang.com/v1/packages/abc/1.20.1.json";
        assert_eq!(
            rewrite_to_bmclapi(url),
            "https://bmclapi2.bangbang93.com/v1/packages/abc/1.20.1.json"
        );
    }

    #[test]
    fn test_rewrite_piston_data() {
        let url = "https://piston-data.mojang.com/v1/objects/abc/client.jar";
        assert_eq!(
            rewrite_to_bmclapi(url),
            "https://bmclapi2.bangbang93.com/v1/objects/abc/client.jar"
        );
    }

    #[test]
    fn test_rewrite_libraries() {
        let url = "https://libraries.minecraft.net/com/mojang/blocklist/1.0/blocklist-1.0.jar";
        assert_eq!(
            rewrite_to_bmclapi(url),
            "https://bmclapi2.bangbang93.com/maven/com/mojang/blocklist/1.0/blocklist-1.0.jar"
        );
    }

    #[test]
    fn test_rewrite_assets() {
        let url = "https://resources.download.minecraft.net/ab/abc123def";
        assert_eq!(
            rewrite_to_bmclapi(url),
            "https://bmclapi2.bangbang93.com/assets/ab/abc123def"
        );
    }

    #[test]
    fn test_third_party_not_rewritten() {
        let url = "https://maven.fabricmc.net/net/fabricmc/fabric-loader/0.16.5/fabric-loader-0.16.5.jar";
        assert_eq!(rewrite_to_bmclapi(url), url);
    }

    #[test]
    fn test_get_urls_mojang() {
        let urls = get_download_urls("https://piston-data.mojang.com/x", Mirror::Mojang);
        assert_eq!(urls.len(), 1);
        assert!(urls[0].contains("mojang.com"));
    }

    #[test]
    fn test_get_urls_bmclapi() {
        let urls = get_download_urls("https://piston-data.mojang.com/x", Mirror::Bmclapi);
        assert_eq!(urls.len(), 1);
        assert!(urls[0].contains("bmclapi2"));
    }

    #[test]
    fn test_get_urls_auto() {
        let urls = get_download_urls("https://piston-data.mojang.com/x", Mirror::Auto);
        assert_eq!(urls.len(), 2);
        assert!(urls[0].contains("bmclapi2"));
        assert!(urls[1].contains("mojang.com"));
    }

    #[test]
    fn test_get_urls_auto_third_party() {
        let urls = get_download_urls("https://maven.fabricmc.net/x", Mirror::Auto);
        assert_eq!(urls.len(), 1);
        assert!(urls[0].contains("fabricmc.net"));
    }
}
