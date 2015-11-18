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

	"os"
	"strconv"

	"./redigo/redis"

	_ "./mysql"
	"database/sql"
	"time"
)

var (
	Pool      *redis.Pool
	MAXFOODID int
	MAXUSERID int
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
	addr := fmt.Sprintf("%s:%s", host, port)

	REDIS_HOST := os.Getenv("REDIS_HOST")
	REDIS_PORT := os.Getenv("REDIS_PORT")
	redis_addr := fmt.Sprintf("%s:%s", REDIS_HOST, REDIS_PORT)
	Pool = newPool(redis_addr, "")

	loadUsersAndFoods()

	InitService(addr)
}

func newPool(server, password string) *redis.Pool {
	return &redis.Pool{
		MaxIdle:     1000,
		IdleTimeout: 240 * time.Second,
		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial("tcp", server)
			if err != nil {
				return nil, err
			}
			//if _, err := c.Do("AUTH", password); err != nil {
			//	c.Close()
			//	return nil, err
			//}
			return c, err
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
	}
}

/**
 * load mysql table user, food to redis
 * @return {[type]} [description]
 */
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

	rs := Pool.Get()
	defer rs.Close()

	var id int
	var name string
	var password string
	for rows.Next() {
		err := rows.Scan(&id, &name, &password)
		if err != nil {
			panic(err.Error())
		}
		//fmt.Printf("%d, %s, %s\n", id, name, password)
		//rs.Do("HSET", "user:"+strconv.Itoa(id), "name", name)
		//rs.Do("HSET", "user:"+strconv.Itoa(id), "password", password)
		rs.Do("HSET", "user:"+name, "id", id)
		rs.Do("HSET", "user:"+name, "password", password)

		if id > MAXUSERID {
			MAXUSERID = id
		}

	}

	rows, err = db.Query("select * from food")
	if err != nil {
		panic(err.Error())
	}

	var stock, price int
	for rows.Next() {
		err = rows.Scan(&id, &stock, &price)
		if err != nil {
			panic(err.Error())
		}
		//fmt.Printf("%d, %s, %s\n", id, name, password)
		rs.Do("HSET", "food:"+strconv.Itoa(id), "stock", stock)
		rs.Do("HSET", "food:"+strconv.Itoa(id), "price", price)

		if id > MAXFOODID {
			MAXFOODID = id
		}

	}

	rows.Close()
}
