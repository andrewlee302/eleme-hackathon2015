package main

// resident memory
var (
	FoodList  []Food                   // index from 1 to FoodNum
	UserList  []User                   // index from 1 to UserNum
	UserMap   map[string]UserIdAndPass // map[name]password
	FoodNum   int
	UserNum   int
	MaxFoodID int
	MaxUserID int

	CacheFoodList    []Food
	CacheCartId      int
	CacheUserOrdered []bool
)
