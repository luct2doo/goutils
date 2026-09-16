package database

import (
	"errors"
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

func (d *Database) DeleteAllTables() (err error) {
	switch d.dbCfg.Connection {
	case "mysql":
		err = d.deleteMySQLTables()
	case "sqlite":
		err = d.deleteAllSqliteTables()
	default:
		panic(errors.New("database connection not supported"))
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

	d.DB.Exec("SET foreign_key_checks = 0;")
	for _, table := range tables {
		err := d.DB.Migrator().DropTable(table)
		if err != nil {
			return err
		}
	}

	d.DB.Exec("SET foreign_key_checks = 1;")
	return nil
}

func (d *Database) TableName(obj any) string {
	stmt := &gorm.Statement{DB: d.DB}
	stmt.Parse(obj)
	return stmt.Schema.Table
}
