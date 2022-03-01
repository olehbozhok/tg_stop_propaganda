package main

import "gorm.io/gorm"

type PropagandaURL struct {
	gorm.Model
	URL      string `gorm:"unique;not null"`
	IsSent bool   `gorm:"index;not null"`
}
