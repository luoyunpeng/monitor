package common

import (
	"database/sql"

	_ "github.com/go-sql-driver/mysql"
)

var (
	db  *sql.DB
	err error

	dbHost     = "localhost:3306"
	dbUser     = "root"
	dbPassword = "123"
	dbName     = "blockchain_db"

	//tableOrder     = "b_order"
	tableContainer = "b_container_service"
)

func InitMysql() error {
	if db != nil {
		return nil
	}
	db, err = sql.Open("mysql", dbUser+":"+dbPassword+"@tcp("+dbHost+")/"+dbName+"?charset=utf8")
	if err != nil {
		return err
	}

	db.SetMaxIdleConns(1)
	db.SetMaxOpenConns(1)
	err = db.Ping()
	if err != nil {
		db.Close()
		return err
	}
	return nil
}

func ChangeContainerStatus(id, status string) error {
	errInit := InitMysql()
	if errInit != nil {
		return errInit
	}
	//c_id is the primary key, update will only affect one row in table b_container_service
	_, err := db.Exec("update "+tableContainer+" set status = ? where c_id = ?", status, id)
	/*
		rowUpdate, err := updateRes.RowsAffected()
		if err != nil {
			return err
		}
		if rowUpdate != 1 {
			return errors.New(strconv.Itoa(int(rowUpdate)) + " row is update please check")
		}*/
	return err
}

func QueryContainerStatus(id string) (int, error) {
	errInit := InitMysql()
	if errInit != nil {
		return -100, errInit
	}
	status := -100
	err = db.QueryRow("SELECT status from "+tableContainer+" where c_id = ?", id).Scan(&status)
	if err != nil {
		/*if err == sql.ErrNoRows{
			no row select
		}else {
			other error
		}*/
		return -100, err
	}

	return status, nil
}

func CloseDB() error {
	if db == nil {
		return nil
	}
	return db.Close()
}
