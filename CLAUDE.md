# GFast V3.2 — Claude Context

Go-based admin/RBAC backend built on GoFrame v2.10, paired with the Vue3 `gfast-ui` frontend (separate repo). PostgreSQL 14+ for storage, Redis for caching and tokens.

## Stack

- **Language:** Go 1.23 (toolchain 1.24)
- **Framework:** GoFrame v2.10 (`github.com/gogf/gf/v2`)
- **DB driver:** `github.com/gogf/gf/contrib/drivers/pgsql/v2` (PostgreSQL only — MySQL was removed in May 2026)
- **Cache/Token:** Redis via `github.com/gogf/gf/contrib/nosql/redis/v2`, gtoken (`github.com/tiger1103/gfast-token`)
- **Auth:** casbin v2 with a custom gdb-backed adapter (`internal/app/common/service/casbin.go`)
- **Other:** base64Captcha, gopsutil, EventBus

## Running locally

```bash
# 1. Create the database
createdb -U postgres token_flux -E UTF8
psql -U postgres -d token_flux -f resource/data/gfast-v32.sql

# 2. Copy the runtime config (the .yaml is gitignored)
cp manifest/config/config.yaml.bak manifest/config/config.yaml
# Edit manifest/config/config.yaml: update database.default.link and redis.default.address as needed

# 3. Run
go run main.go      # serves on :8808
```

Default seed account: `demo` / `123456` (the login API requires a captcha — fetch via `/api/v1/pub/captcha/get`).

## Repository layout

```
main.go                          # entry point, imports pgsql driver + system packed + boot
internal/
  cmd/cmd.go                     # gcmd.Command starts gf server, mounts router, binds OpenAPI
  app/
    boot/                        # init wiring (loaded via blank import in main.go)
    common/                      # shared services: casbin enforcer + watcher, common dao/model
    system/                      # the system module (users, roles, depts, menus, dict, config, logs)
      api/  (under repo /api/v1) # request/response types with goframe path tags
      controller/                # thin HTTP handlers
      logic/                     # business logic (one package per domain, e.g. sysUser, sysRole)
      service/                   # interface registration (gf convention: logic → service)
      dao/                       # generated DAOs (gen via `gf gen dao`, config in hack/config.yaml)
      model/                     # entity (DB row) + do (insert/update) + custom DTO
      router/                    # route registration
      consts/                    # constants
  mounter/                       # component mounting
  router/                        # top-level router
api/v1/system/                   # API request/response definitions (path-tagged for OpenAPI)
library/                         # libResponse, libRouter, libUtils, liberr — shared helpers
resource/
  data/gfast-v32.sql             # PostgreSQL 14+ init script (14 tables + seed data)
  casbin/                        # RBAC model and policy files
  public/                        # static assets
  log/                           # runtime logs (gitignored)
manifest/
  config/config.yaml.bak         # config template (real config.yaml is local & gitignored)
  docker/Dockerfile              # minimal alpine image, copies bin + resource
  deploy/kustomize/              # K8s overlays (only server + logger; DB config injected at runtime)
hack/config.yaml                 # gfcli dao-gen config (pgsql connection)
docs/superpowers/                # specs + plans for non-trivial features (current: pgsql migration)
```

## Architectural conventions

- **Module layering:** controller → logic → dao. Controllers stay thin (parse request, call logic, return).
- **Service registration:** GoFrame convention — `logic` packages register themselves into `service` interfaces via `init()`. Always import the logic package somewhere on the call path so `init` runs.
- **DAO generation:** Don't hand-edit `dao/internal/*.go` — regenerate via `gf gen dao` using `hack/config.yaml`.
- **Entity vs DO vs Model:** `entity/` mirrors DB rows; `do/` is for `Insert`/`Update` payloads (omit-zero semantics); free-form DTOs live in `model/`.
- **DB access:** Use `dao.X.Ctx(ctx)` — never raw SQL unless absolutely necessary. Two existing `g.DB().Ctx(ctx).Exec(ctx, "truncate ...")` calls are the only deviations.
- **Transactions:** `g.DB().Transaction(ctx, func(ctx, tx gdb.TX) error {...})` — already used in user/role/dept/dict/auth logic.
- **Casbin:** Single enforcer via `service.CasbinEnforcer()` (sync.Once). Adapter persists to the `casbin_rule` table (with synthetic `id` PK as of May 2026). Optional cluster mode broadcasts policy reloads through Redis (`casbin_policy_channel`).
- **Auth tokens:** gfast-token (gtoken-derived) — exclusions configured in `gfToken.excludePaths`.
- **OpenAPI:** Auto-derived from request struct tags. Swagger at `/swagger`, JSON at `/api.json`.

## Working in this codebase

- **Default branch:** `os-v3.2` (this is also the PR target).
- **Frontend lives elsewhere:** https://github.com/tiger1103/gfast-ui — don't try to find UI code in this repo.
- **Tests:** there isn't a test suite. Verification is via running the server and exercising endpoints.
- **Logs:**
  - App logs: `resource/log/run/<date>.log`
  - HTTP access logs: `resource/log/server/<date>.log`
  - SQL trace: `resource/log/sql/<date>.log` (enabled when `database.logger` is configured)
- **DB dialect quirks:**
  - All identifiers are unquoted lower snake_case — PG folds them to lowercase automatically. Keep it that way.
  - Auto-increment columns use `GENERATED BY DEFAULT AS IDENTITY` so seed scripts can supply explicit ids; each seeded table ends with a `setval(pg_get_serial_sequence(...), MAX(...))` to keep the sequence in sync.
  - `tinyint` from old MySQL maps to `smallint`; gdb decodes it transparently to `int`/`bool`. No code changes needed.
- **When the schema changes:** update `resource/data/gfast-v32.sql` directly (no migration framework). Regenerate DAOs with `gf gen dao` if you add/modify tables.
- **Don't add backwards-compat shims for MySQL.** The migration is complete and MySQL support is intentionally removed.

## 数据库迁移 (Goose)

迁移文件位于 `resource/migrations/`，通过 `//go:embed` 嵌入二进制。

### 自动迁移
- 启动时默认执行 `goose Up`，由 `database.migration.autoMigrate` 开关控制
- 配置 `database.migration.autoMigrate: false` 可关闭自动迁移
- 迁移失败会 `Fatal` 退出（schema 不一致比服务不可用更危险）
- 迁移版本表：`goose_db_version`（可通过 `database.migration.tableName` 自定义）

### CLI 手动操作
```bash
go run main.go migrate up              # 执行所有待运行的迁移
go run main.go migrate down            # 回退最近一个迁移
go run main.go migrate status          # 查看当前版本状态
go run main.go migrate create <name> sql  # 创建新迁移文件
```

### 新建迁移
1. `go run main.go migrate create add_xxx sql` → 在 `resource/migrations/` 下生成 `NNNNN_add_xxx.sql`
2. 填写 `-- +goose Up` 和 `-- +goose Down` 块
3. 重新编译（embed 自动包含新文件）
