/*

APP_HOST=0.0.0.0
APP_PORT=8080

DB_HOST=localhost
DB_PORT=3306
DB_NAME=eleme
DB_USER=root
DB_PASS=toor

REDIS_HOST=localhost
REDIS_PORT=6379

PYTHONPATH=/vagrant
GOPATH=/srv/gopath
JAVA_HOME=/usr/lib/jvm/java-8-openjdk-amd64

*/

package main

import (
	"fmt"
	"net/http"
	"os"

	_ "./mysql"
	"database/sql"
)

func main() {
	host := os.Getenv("APP_HOST")
	port := os.Getenv("APP_PORT")
	if host == "" {
		host = "localhost"
	}
	if port == "" {
		port = "8080"
	}
	//addr := fmt.Sprintf("%s:%s", host, port)

	loadUsersAndFoods()

	//http.HandleFunc("/login", login)

	//http.ListenAndServe(addr, nil)
}

func loadUsersAndFoods() {

	DB_HOST := os.Getenv("DB_HOST")
	DB_PORT := os.Getenv("DB_PORT")
	DB_NAME := os.Getenv("DB_NAME")
	DB_USER := os.Getenv("DB_USER")
	DB_PASS := os.Getenv("DB_PASS")
	mysql_addr := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", DB_USER, DB_PASS, DB_HOST, DB_PORT, DB_NAME)

	db, err := sql.Open("mysql", mysql_addr)
	if err != nil {
		panic(err.Error())
	}
	defer db.Close()

	rows, err := db.Query("select * from user")
	if err != nil {
		panic(err.Error())
	}
	defer rows.Close()

	err = db.Ping()
	if err != nil {
		panic(err.Error())
	}

	fmt.Println("wulang")

	var id int
	var name string
	var password string
	for rows.Next() {
		err := rows.Scan(&id, &name, &password)
		if err != nil {
			panic(err.Error())
		}
		fmt.Printf("%d, %s, %s\n", id, name, password)
	}

}

func login(w http.ResponseWriter, r *http.Request) {
	//username := r.PostFormValue("username")
	//password := r.PostFormValue("password")

}
