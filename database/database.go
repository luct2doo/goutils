package database

import (
	"fmt"

	"github.com/luct2doo/goutils/config"

	"gorm.io/gorm"
)

type Database struct {
	DB    *gorm.DB
	dbCfg *config.Database
}

func NewDatabase(db *gorm.DB, dbCfg *config.Database) *Database {
	return &Database{
		DB:    db,
		dbCfg: dbCfg,
	}
}

func (d *Database) CurrentDatabase() string {
	return d.DB.Migrator().CurrentDatabase()
}

// DeleteAllTables 删除当前连接所连库中的全部表。
//
// ⚠️ 破坏性操作，请仅在测试环境中调用。
// 连接类型不支持时返回 error（不再 panic），避免库代码直接终结调用方进程。
func (d *Database) DeleteAllTables() (err error) {
	switch d.dbCfg.Connection {
	case "mysql":
		err = d.deleteMySQLTables()
	case "sqlite":
		err = d.deleteAllSqliteTables()
	default:
		return fmt.Errorf("database connection not supported: %q", d.dbCfg.Connection)
	}
	return err
}

func (d *Database) deleteAllSqliteTables() error {
	var tables []string

	err := d.DB.Select(&tables, "SELECT name FROM sqlite_master WHERE type='table'").Error
	if err != nil {
		return err
	}

	for _, table := range tables {
		err := d.DB.Migrator().DropTable(table)
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *Database) deleteMySQLTables() error {
	dbName := d.CurrentDatabase()
	var tables []string

	err := d.DB.Table("information_schema.tables").
		Where("table_schema = ?", dbName).
		Pluck("table_name", &tables).Error
	if err != nil {
		return err
	}

	// 无论中途是否失败，都要把 foreign_key_checks 恢复：
	// 连接会被连接池复用，残留 =0 会让后续业务写入跳过外键约束。
	d.DB.Exec("SET foreign_key_checks = 0;")
	defer func() {
		d.DB.Exec("SET foreign_key_checks = 1;")
	}()

	for _, table := range tables {
		err := d.DB.Migrator().DropTable(table)
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *Database) TableName(obj any) string {
	stmt := &gorm.Statement{DB: d.DB}
	stmt.Parse(obj)
	return stmt.Schema.Table
}
