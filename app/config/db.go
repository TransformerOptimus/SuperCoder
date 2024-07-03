package config

import (
	"fmt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"log"
)

func DBHost() string {
	return config.String("db.host")
}

func DBUser() string {
	return config.String("db.user")
}

func DBPassword() string {
	return config.String("db.password")
}

func DBName() string {
	return config.String("db.name")
}

func DBPort() int {
	return config.Int("db.port")
}

func AppEnv() string { return config.String("app.env") }

func DatabaseDSN() string {

	dns := "host=%s user=%s password=%s dbname=%s port=%d"
	if AppEnv() == "development" {
		dns += " sslmode=disable"
	}
	return fmt.Sprintf(dns, DBHost(), DBUser(), DBPassword(), DBName(), DBPort())
}

var psqlDB *gorm.DB

func InitDB() *gorm.DB {
	dsn := DatabaseDSN()
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("could not connect to the database: %v", err)
	}

	psqlDB = db
	return psqlDB
}

func CloseDB() {
	sqlDB, err := psqlDB.DB()
	if err != nil {
		log.Fatalf("could not get database object from Gorm DB: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		log.Fatalf("could not close database connection: %v", err)
	}
}
