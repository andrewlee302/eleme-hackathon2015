package main

type Food struct {
	Id    int `json:"id"`
	Price int `json:"price"`
	Stock int `json:"stock"`
}

type LoginJson struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type CartWL struct {
	total  int
	userId int
	Items  []CartItem
}

// Cart can be used as a order
type Cart struct {
	// if total < 0, then is a order
	Id         string     `json:"id"`
	Items      []CartItem `json:"items"`
	TotalPrice int        `json:"total"`
}

// for GET /admin/orders
type CartDetail struct {
	Id         string     `json:"id"`
	UserId     int        `json:"user_id"`
	Items      []CartItem `json:"items"`
	TotalPrice int        `json:"total"`
}

type CartItem struct {
	FoodId int `json:"food_id"`
	Count  int `json:"count"`
}

type CartIdJson struct {
	CartId string `json:"cart_id"`
}

type UserIdAndPass struct {
	Id       string
	Password string
}
