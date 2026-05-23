package boot

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/glog"
	"github.com/pressly/goose/v3"
	"github.com/tiger1103/gfast/v3/resource/migrations"
)

// AutoMigrate 在应用启动时自动执行数据库迁移。
// 读取 database.migration.autoMigrate 配置，若为 true 则执行 goose Up。
// 迁移失败直接 Fatal 退出（schema 不一致比服务不可用更危险）。
func AutoMigrate(ctx context.Context) {
	autoMigrate := g.Cfg().MustGet(ctx, "database.migration.autoMigrate", true).Bool()
	if !autoMigrate {
		glog.Info(ctx, "[migration] autoMigrate is disabled, skip")
		return
	}

	tableName := g.Cfg().MustGet(ctx, "database.migration.tableName", "goose_db_version").String()

	db, err := g.DB().GetCore().Master()
	if err != nil {
		g.Log().Fatal(ctx, "[migration] failed to get sql.DB:", err)
	}

	provider, err := goose.NewProvider(
		goose.DialectPostgres,
		db,
		migrations.FS,
		goose.WithTableName(tableName),
	)
	if err != nil {
		g.Log().Fatal(ctx, "[migration] failed to create goose provider:", err)
	}

	if _, err := provider.Up(ctx); err != nil {
		g.Log().Fatal(ctx, "[migration] goose Up failed:", err)
	}

	glog.Info(ctx, "[migration] goose Up completed successfully")
}
