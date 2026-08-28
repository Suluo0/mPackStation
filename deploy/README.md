# 单机部署说明

本目录描述 mPackStation 的单实例分发和部署边界。当前目标是本机运行；默认只监听 `127.0.0.1`，不包含数据库、缓存、JAR、导出物或任何密钥。

## 分发包内容

由 `scripts/package.ps1` 生成的目录至少包含：

```text
mpackstation.exe
web/
VERSION
BUILD.txt
README.txt
LICENSE
```

运行时数据必须放在用户明确指定的绝对目录中，例如：

```powershell
./mpackstation.exe -addr 127.0.0.1:18871 -data 'C:/Users/<user>/AppData/Local/mPackStation/data'
```

首次运行会初始化数据库目录；部署脚本不得覆盖已有数据，也不得删除数据目录。卸载只移除程序文件，用户必须显式确认后才能清理数据。

## 升级原则

1. 停止由本次安装启动的服务。
2. 备份 SQLite 数据库和配置元数据到用户指定位置。
3. 替换程序文件，保留数据目录。
4. 启动新版本并等待 `/api/readyz` 返回就绪。
5. 若迁移或检查失败，保留现场并阻止继续提供业务服务。

当前后端仍处于骨架阶段，完整 migration runner、业务恢复和静态资源服务完成前，不能把本 smoke 流程当作正式生产升级证明。

