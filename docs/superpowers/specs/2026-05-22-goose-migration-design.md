# Goose 数据库迁移集成设计

## 背景

GFast V3.2 当前使用单一 SQL 文件 `resource/data/gfast-v32.sql`（559 行，14 张表 + 种子数据）做数据库初始化，无版本管理。引入 `pressly/goose` 建立迁移体系，支持增量 schema 变更和版本追踪。

## 决策记录

| 项目 | 决策 |
|------|------|
| 调用方式 | 嵌入式自动迁移 + CLI 备用 + `autoMigrate` 配置开关 |
| 既有 SQL 处理 | 单个 `00001_init_schema.sql` 包装现有全部内容 |
| 迁移文件目录 | `resource/migrations/` |
| goose 引入方式 | 只引入库，自建 `migrate` gcmd 子命令 |
| 配置结构 | `database.migration` 下 `autoMigrate`/`dir`/`tableName` |
| 迁移失败行为 | 失败直接 `g.Log().Fatal()` 退出 |
| 旧 SQL 文件 | 删除 `resource/data/gfast-v32.sql` |

## 改动文件清单

| # | 文件 | 改动 |
|---|------|------|
| 1 | `resource/migrations/00001_init_schema.sql` | 新建，包装现有 14 张表 + 种子数据 |
| 2 | `resource/data/gfast-v32.sql` | 删除 |
| 3 | `internal/cmd/migrate.go` | 新建，注册 `migrate` 子命令（up/down/status/create） |
| 4 | `internal/app/boot/migrate.go` | 新建，启动时自动 `goose.Up()` 逻辑 |
| 5 | `manifest/config/config.yaml.bak` | 加 `database.migration` 配置段 |
| 6 | `go.mod` | 加 `github.com/pressly/goose/v3` |
| 7 | `CLAUDE.md` | 更新迁移相关说明 |
| 8 | `README.MD` | 更新数据库初始化说明 |

## 核心流程

### 应用启动时

```
main.go → boot.Init() → 读取 database.migration.autoMigrate
  → true: goose.Up(db, migrationsFS, goose.WithTableName(tableName))
  → 失败: g.Log().Fatal() 退出
  → false: 跳过
```

### CLI 手动

```bash
go run main.go migrate up                          # 执行所有待运行的迁移
go run main.go migrate down                        # 回退一个版本
go run main.go migrate status                      # 查看当前版本
go run main.go migrate create add_xxx_column sql   # 创建新迁移文件
```

### 新建迁移

`go run main.go migrate create add_foo_column sql` → 在 `resource/migrations/` 下生成 `NNNNN_add_foo_column.sql`，开发者填写 `-- +goose Up` / `-- +goose Down` 块。

## 嵌入方式

迁移文件通过 `//go:embed resource/migrations/*.sql` 嵌入二进制，部署时无需额外文件。`create` 子命令生成的文件直接写磁盘（开发期用），下一次编译自动包含。

## 配置结构

```yaml
database:
  default:
    link: "pgsql:postgres:123456@tcp(localhost:5432)/token_flux?sslmode=disable"
    debug: true
    dryRun: false
    maxIdle: 10
    maxOpen: 10
    maxLifetime: "30s"
  migration:
    autoMigrate: true
    dir: "resource/migrations"
    tableName: "goose_db_version"
```

- `autoMigrate: true` 默认开启，生产环境想关掉改为 `false`
- `dir` 供 CLI `create` 子命令写入磁盘使用；嵌入式运行时不依赖此路径（用 embed.FS）
- `tableName` 传给 `goose.WithTableName()`，默认 `goose_db_version`

## `00001_init_schema.sql` 结构

```sql
-- +goose Up
-- +goose StatementBegin

-- 现有 559 行 SQL 全部放在这里（CREATE TABLE + INSERT + setval）
-- 内容来自当前 resource/data/gfast-v32.sql

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS sys_user_post;
DROP TABLE IF EXISTS sys_user_online;
DROP TABLE IF EXISTS sys_user;
DROP TABLE IF EXISTS sys_role_dept;
DROP TABLE IF EXISTS sys_role;
DROP TABLE IF EXISTS sys_post;
DROP TABLE IF EXISTS sys_oper_log;
DROP TABLE IF EXISTS sys_login_log;
DROP TABLE IF EXISTS sys_dict_type;
DROP TABLE IF EXISTS sys_dict_data;
DROP TABLE IF EXISTS sys_dept;
DROP TABLE IF EXISTS sys_config;
DROP TABLE IF EXISTS sys_auth_rule;
DROP TABLE IF EXISTS casbin_rule;

-- +goose StatementEnd
```

Down 块按依赖倒序 DROP 14 张表。

## `internal/cmd/migrate.go` 设计

注册 `migrate` gcmd 子命令，包含 4 个子操作：

- `up`：调用 `goose.Up(db, migrations, goose.WithTableName(cfg.TableName))`
- `down`：调用 `goose.Down(db, migrations, goose.WithTableName(cfg.TableName))`
- `status`：调用 `goose.Status(db, migrations, goose.WithTableName(cfg.TableName))`
- `create <name> sql`：调用 `goose.Create(db, cfg.Dir, name, "sql")` — 注意 create 必须写磁盘，不使用 embed.FS

所有子操作共用一个获取 `*sql.DB` 连接的 helper：从 GoFrame `g.DB()` 获取底层 `*sql.DB`。

## `internal/app/boot/migrate.go` 设计

`AutoMigrate(ctx context.Context)` 函数：

1. 读 `database.migration.autoMigrate` 配置
2. 若 `false` 或未配置，直接 return
3. 从 `g.DB()` 获取底层 `*sql.DB`
4. `goose.SetBaseFS(migrationsFS)` 设置嵌入的迁移文件系统
5. `goose.Up(db, "", goose.WithTableName(cfg.TableName))` — 空字符串表示从 BaseFS 根目录读取
6. 失败则 `g.Log().Fatal(ctx, "migration failed:", err)`

在现有 boot 初始化流程中调用此函数（在 DB 连接建立之后、路由注册之前）。

## 不需要改动的部分

- 业务 logic/dao/model/entity：零改动
- casbin adapter：零改动
- Docker / K8s 配置：零改动
- `hack/config.yaml`：不涉及（gf gen dao 仍直连数据库读 schema）

## 风险评估

| 风险 | 程度 | 缓解 |
|------|------|------|
| 首次 goose Up 时 DB 已有表（重复创建） | 中 | 首次部署到已有库时需先手动插入 `goose_db_version` 记录标记版本 1，或全新建库 |
| `goose.Up()` 耗时影响启动速度 | 低 | PG 14 张表初始化 < 1s；增量迁移通常 < 100ms |
| `create` 子命令写磁盘 vs embed.FS 不一致 | 低 | 开发期正常流程：create → 编译 → up；CI 构建自动包含新文件 |
| 迁移失败 Fatal 导致 Pod 崩溃循环 | 低 | 这是期望行为——schema 不一致比服务不可用更危险 |
