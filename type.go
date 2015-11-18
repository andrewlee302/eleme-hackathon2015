package main

type Food struct {
	Id    int `json:"id"`
	Price int `json:"price"`
	Stock int `json:"stock"`
}

type User struct {
	Id             int
	Name, Password string
}

type LoginJson struct {
	Username string `json:"username"`
	Password string `json:"password"`
}
