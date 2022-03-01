package main

import (
	"fmt"
	"net/url"

	"github.com/pkg/errors"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func initDB(user, password, host, dbName string) (db *gorm.DB, err error) {
	password = url.QueryEscape(password)
	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8mb4&parseTime=True&loc=Local", user, password, host, dbName)
	db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, errors.Wrap(err, "error opening database")
	}
	db.CreateBatchSize = 300

	err = db.AutoMigrate(&PropagandaURL{})
	if err != nil {
		return nil, errors.Wrap(err, "error AutoMigrate")
	}

	return db, nil
}
