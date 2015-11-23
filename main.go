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

// go run  main.go service.go type.go

package main

import (
	_ "./mysql"
	"./redigo/redis"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"
)

var (
	Pool *redis.Pool
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
		MaxIdle:     9000,
		IdleTimeout: 666 * time.Second,
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

	rs := Pool.Get()
	defer rs.Close()
	rs.Do("SET", "cart_id", 0)

	// Load LuaScript
	LuaAddFood.Load(rs)
	LuaSubmitOrder.Load(rs)

	db, err := sql.Open("mysql", mysql_addr)
	if err != nil {
		panic(err.Error())
	}
	defer db.Close()

	rows, err := db.Query("SELECT COUNT(*) FROM food")
	if err != nil {
		panic(err.Error())
	}
	if rows.Next() {
		rows.Scan(&FoodNum)
	}
	rows.Close()

	rows, err = db.Query("SELECT COUNT(*) FROM user")
	if err != nil {
		panic(err.Error())
	}
	if rows.Next() {
		rows.Scan(&UserNum)
	}
	rows.Close()
	FoodList = make([]Food, FoodNum+1)
	CacheFoodList = make([]Food, FoodNum+1)
	UserList = make([]User, UserNum+1)
	UserMap = make(map[string]UserIdAndPass)

	rows, err = db.Query("select * from user")
	if err != nil {
		panic(err.Error())
	}

	var userId int
	var name, password string
	cnt := 1
	for rows.Next() {
		err := rows.Scan(&userId, &name, &password)
		if err != nil {
			panic(err.Error())
		}
		UserList[cnt].Id = userId
		UserList[cnt].Name = name
		UserList[cnt].Password = password
		UserMap[name] = UserIdAndPass{strconv.Itoa(userId), password}
		cnt++
		// rs.Do("HMSET", "user:"+name, "id", userId, "password", password)
		if userId > MaxUserID {
			MaxUserID = userId
		}

	}
	rows.Close()
	rows, err = db.Query("select * from food")
	if err != nil {
		panic(err.Error())
	}

	var foodId, stock, price int
	cnt = 1
	for rows.Next() {
		err = rows.Scan(&foodId, &stock, &price)
		if err != nil {
			panic(err.Error())
		}
		FoodList[cnt].Id = foodId
		FoodList[cnt].Price = price
		FoodList[cnt].Stock = stock

		CacheFoodList[cnt].Id = foodId
		CacheFoodList[cnt].Price = price
		CacheFoodList[cnt].Stock = stock

		cnt++
		rs.Do("HMSET", "food:"+strconv.Itoa(foodId), "stock", stock, "price", price)
		if foodId > MaxFoodID {
			MaxFoodID = foodId
		}
	}
	rows.Close()

	// wl, _ := json.Marshal(CacheFoodList[1:])
	// fmt.Println(len(wl))
	CacheFoodJson = make([]byte, 3370)
	CacheFoodJson, _ = json.Marshal(CacheFoodList[1:])

}
