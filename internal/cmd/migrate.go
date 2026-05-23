package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcmd"
	"github.com/pressly/goose/v3"
	"github.com/tiger1103/gfast/v3/resource/migrations"
)

var (
	Migrate = gcmd.Command{
		Name:  "migrate",
		Usage: "migrate",
		Brief: "database migration management (goose)",
		Func: func(ctx context.Context, parser *gcmd.Parser) (err error) {
			action := parser.GetArg(2).String()
			if action == "" {
				fmt.Println("Usage: go run main.go migrate <up|down|status|create> [name]")
				fmt.Println("  up      - run all pending migrations")
				fmt.Println("  down    - roll back the most recent migration")
				fmt.Println("  status  - show current migration version")
				fmt.Println("  create  - create a new migration file (requires name argument)")
				return nil
			}

			tableName := g.Cfg().MustGet(ctx, "database.migration.tableName", "goose_db_version").String()
			dir := g.Cfg().MustGet(ctx, "database.migration.dir", "resource/migrations").String()

			db, err := g.DB().GetCore().Master()
			if err != nil {
				return fmt.Errorf("get sql.DB: %w", err)
			}

			// up/down/status use SetBaseFS + SetTableName for embedded migrations
			goose.SetBaseFS(migrations.FS)
			goose.SetTableName(tableName)

			switch action {
			case "up":
				return goose.Up(db, ".")
			case "down":
				return goose.Down(db, ".")
			case "status":
				return goose.Status(db, ".")
			case "create":
				name := parser.GetArg(3).String()
				if name == "" {
					return fmt.Errorf("create requires a migration name, e.g.: migrate create add_foo_column sql")
				}
				migrationType := parser.GetArg(4).String()
				if migrationType == "" {
					migrationType = "sql"
				}
				// create writes to disk, not embed.FS
				if err := os.MkdirAll(dir, 0755); err != nil {
					return fmt.Errorf("create migrations dir: %w", err)
				}
				return goose.Create(db, dir, name, migrationType)
			default:
				return fmt.Errorf("unknown action: %s (expected up, down, status, or create)", action)
			}
		},
	}
)

func init() {
	if err := Main.AddCommand(&Migrate); err != nil {
		panic(err)
	}
}
