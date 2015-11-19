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

// Cart can be used as a order
type Cart struct {
	// if total < 0, then is a order
	Total int        `json:"total"`
	Items []CartItem `json:"items"`
}

type CartItem struct {
	FoodId string `json:"food_id"`
	Count  int    `json:"count"`
}
